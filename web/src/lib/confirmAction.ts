// The one way a screen asks "are you sure?" and then shows the work it
// started (owner, 6 Nov 2026).
//
// Deleting a domain showed the browser's own dialogue ("<host>:1923 says —
// Delete …? OK / Cancel"). The owner: "If a message is needed, we must show it
// ourselves." Then, as a firm order: while work runs the person must never
// wait blankly; lock the page and, like the update screen, say what is being
// done, step by step, until it ends. Every confirmation goes through
// confirmAction, which the ConfirmHost mounted at the application root answers
// with the product's own dialogue. A confirmed `action` turns the same
// dialogue into the progress window: the page stays locked (no other control,
// no Escape, no backdrop) and shows the steps the action reports, or one
// honest "Working on the server…" step with the elapsed time, until the action
// ends; then "Completed" or the reason, who acts and the next step.
// window.confirm/alert/prompt are not used anywhere in web/src
// (tests/no-native-dialogs.test.mjs).
//
// One question at a time: while a question waits for its answer, a second
// request is answered "no" at once, so a double click can never queue a second
// change. A running action may ask one follow-up question of its own (the
// server asked for something more); it is shown above the progress window.
// With no host mounted nothing is asked and nothing is done (answer "no").
//
// Bir ekranın "emin misiniz?" diye sormasının ve başlattığı işi göstermesinin
// tek yolu. Onaylanan iş, aynı pencereyi ilerleme penceresine çevirir: sayfa
// kilitlidir, adımlar ya da "Sunucuda çalışılıyor…" ve geçen süre gösterilir;
// sonunda "Tamamlandı" ya da neden, kimin ne yapacağı söylenir.

/** One step of a running operation, as the action reports it. */
export interface OperationStep {
    id: string;
    /** Already in the interface language. */
    label: string;
    /** waiting: the step waits for someone or something else (named in `note`). */
    state: 'done' | 'running' | 'waiting' | 'pending' | 'failed';
    /** When the current state began (ms since the epoch): the window shows the elapsed time. */
    since?: number;
    /** One plain sentence under the step: what it waits for, or why it stopped. */
    note?: string;
}

/**
 * How an operation ended (D-024): done, a verified failure, an unmet
 * prerequisite or someone else must act (blocked), or a result that is not
 * known. `reason` already says who acts and what happens next.
 */
export type OperationEnd =
    | { state: 'done'; message?: string }
    | { state: 'failed' | 'blocked' | 'unknown'; reason: string; fix?: { label: string; run: () => void } };

/** What a running action can tell the window. */
export interface OperationReport {
    /** Replaces the steps shown. */
    steps(steps: OperationStep[]): void;
}

export interface ConfirmRequest {
    /** Short heading, e.g. "Delete example.com?". Without one the message is the dialogue's text. */
    title?: string;
    /** What happens, in plain sentences. Line breaks are kept. */
    message?: string;
    /** The confirm button's label; "Continue" when not given. */
    confirmLabel?: string;
    /** danger: the change removes or overwrites something. */
    tone?: 'default' | 'danger';
    /** The progress window's title while the action runs, e.g. "Deleting example.com". */
    progressTitle?: string;
    /**
     * Runs after the confirm button is pressed, inside the locked progress
     * window. It returns how the operation ended; without an answer (void) the
     * window closes when it ends and the page says the result itself.
     * confirmAction then answers true.
     */
    action?: (report: OperationReport) => Promise<OperationEnd | void>;
}

/** An operation already running, followed without a question (e.g. after a reload). */
export interface FollowRequest {
    progressTitle: string;
    tone?: 'default' | 'danger';
    action: (report: OperationReport) => Promise<OperationEnd | void>;
}

type ConfirmHost = (request: ConfirmRequest & { follow?: boolean }) => Promise<boolean>;

let host: ConfirmHost | null = null;

/** The application root registers its dialogue; returns the unregister. */
export function registerConfirmHost(next: ConfirmHost): () => void {
    host = next;
    return () => {
        if (host === next) host = null;
    };
}

/** Asks the question in the product's own dialogue; true when confirmed. */
export function confirmAction(request: ConfirmRequest): Promise<boolean> {
    return host ? host(request) : Promise.resolve(false);
}

/** Shows the progress window for an operation that is already running. */
export function followOperation(request: FollowRequest): Promise<boolean> {
    return host ? host({ ...request, follow: true }) : Promise.resolve(false);
}
