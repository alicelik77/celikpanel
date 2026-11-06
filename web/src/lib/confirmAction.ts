// The one way a screen asks "are you sure?" (owner report, 6 Nov 2026).
//
// Deleting a domain showed the browser's own dialogue ("<host>:1923 says —
// Delete …? OK / Cancel"). The owner: "If a message is needed, we must show it
// ourselves." Every confirmation now goes through confirmAction, which the
// ConfirmHost mounted at the application root answers with the product's own
// dialogue. window.confirm/alert/prompt are not used anywhere in web/src
// (tests/no-native-dialogs.test.mjs).
//
// One question at a time: while a confirmation is open, a second request is
// answered "no" at once, so a double click can never queue a second change.
// With no host mounted nothing is asked and nothing is done (answer "no").
//
// Bir ekranın "emin misiniz?" diye sormasının tek yolu (sahip, 6 Kas 2026).
// Tarayıcının kendi penceresi artık kullanılmaz; soru ürünün kendi penceresidir.
// Aynı anda tek soru: açık bir soru varken gelen ikinci istek hemen "hayır"
// yanıtını alır; çift tıklama ikinci bir değişiklik kuyruğa koyamaz.

export interface ConfirmRequest {
    /** Short heading, e.g. "Delete example.com?". Without one the message is the dialogue's text. */
    title?: string;
    /** What happens, in plain sentences. Line breaks are kept. */
    message?: string;
    /** The confirm button's label; "Continue" when not given. */
    confirmLabel?: string;
    /** danger: the change removes or overwrites something. */
    tone?: 'default' | 'danger';
    /** Runs after the confirm button is pressed; the button shows it is busy
        and the dialogue stays until this ends. confirmAction then answers true. */
    action?: () => Promise<unknown>;
}

type ConfirmHost = (request: ConfirmRequest) => Promise<boolean>;

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
