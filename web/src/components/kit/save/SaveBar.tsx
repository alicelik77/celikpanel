import { useI18n } from '../../../i18n';
import { Button } from '../../ui';
import { useSaveWords } from './words';

// The one save bar of every settings screen (alpha.132, drawings X16, D10,
// V18): "N changes not saved. Nothing changes until you press Save." with
// Discard and Save. It shows only while something is unsaved (or a save runs,
// or a save's answer was lost). Save is off, with the reason in place of the
// count, while saving is not possible: the screen's own reason (`blockedBy`:
// what is shown is an earlier answer, a field is missing...) or a lost answer
// that has not been checked yet. While a save runs both buttons are locked.
//
// It is the last child of its card and sticks to the bottom of the window
// while the card is longer than the window, on a desk and on a phone; it takes
// its own room in the card, so at rest it covers nothing. A card with its own
// padding passes the same padding as a negative margin (`className`, e.g.
// "-mx-5 px-5") so the bar's line runs edge to edge.
//
// Her ayar ekranının tek kaydet çubuğu: "N değişiklik kaydedilmedi. Kaydet'e
// basana dek hiçbir şey değişmez." + Vazgeç + Kaydet. Yalnız kaydedilmemiş bir
// şey varken görünür; kaydetmek mümkün değilken Kaydet kapalıdır ve nedeni
// sayının yerinde yazar. Kartın son çocuğudur, pencerenin altına yapışır.

export interface SaveBarProps {
    /** How many changes are unsaved; 'some' when they cannot be counted (a text that differs). */
    count: number | 'some';
    saving: boolean;
    onSave: () => void;
    onDiscard: () => void;
    /** Why Save is off although something changed; said in place of the count. */
    blockedBy?: string;
    /**
     * The last save's answer was lost: the bar says so, Save is off while
     * `held` (the server has not been read again), and "Check again" reads.
     */
    unknown?: { held: boolean; checking: boolean; onCheck: () => void };
    className?: string;
}

export function SaveBar({ count, saving, onSave, onDiscard, blockedBy, unknown, className = '' }: SaveBarProps) {
    const { t } = useI18n();
    const say = useSaveWords();
    if (count === 0 && !saving && !unknown) return null;
    // While a save runs the bar says so, never a count that is already
    // going down; it closes once the save has ended.
    const line = saving
        ? say('savingNow')
        : unknown
        ? say('unknown')
        : blockedBy || (count === 'some' ? say('pendingSome') : count === 1 ? say('pendingOne') : say('pending', { n: count }));
    return (
        <div
            data-save-bar=""
            className={`sticky bottom-0 z-10 mt-4 flex flex-wrap items-center justify-end gap-x-3 gap-y-2 border-t border-border bg-surface py-3 ${className}`}
        >
            <p className="flex min-w-0 basis-full items-start gap-2 text-sm text-fg sm:mr-auto sm:basis-0 sm:grow" aria-live="polite">
                <span className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${!saving && (unknown || blockedBy) ? 'bg-warning' : 'bg-primary'}`} aria-hidden="true" />
                <span className="min-w-0 max-w-[75ch]">{line}</span>
            </p>
            {unknown && (
                <Button type="button" onClick={unknown.onCheck} loading={unknown.checking} data-save-check="">
                    {t('common.checkAgain')}
                </Button>
            )}
            <Button type="button" onClick={onDiscard} disabled={count === 0 || saving} data-save-discard="">
                {say('discard')}
            </Button>
            <Button
                type="button"
                variant="primary"
                onClick={onSave}
                disabled={count === 0 || !!blockedBy || !!unknown?.held}
                loading={saving}
                data-save=""
            >
                {saving ? say('saving') : say('save')}
            </Button>
        </div>
    );
}

/**
 * The mark beside a changed setting's name: "Changed"; "Changed: off → on"
 * for a switch (`switched` is its new state); or "Changed: {from} → {to}"
 * when the screen gives both values in words.
 */
export function ChangedMark({ from, to, switched }: { from?: string; to?: string; switched?: boolean }) {
    const say = useSaveWords();
    const pair = switched === undefined ? (from !== undefined && to !== undefined ? { from, to } : null)
        : { from: say(switched ? 'off' : 'on'), to: say(switched ? 'on' : 'off') };
    return (
        <span data-changed-mark="" className="ml-2 inline-block rounded bg-primary/10 px-1.5 py-0.5 align-middle text-xs font-medium text-primary">
            {pair ? say('changedFrom', pair) : say('changed')}
        </span>
    );
}

/** The row of a changed setting: a line on its leading edge and a light wash (drawing X16). */
export const changedRowClass = 'border-l-2 border-l-primary bg-primary/5';

/**
 * Under a setting the person changed and the server changed too while they
 * were editing: the server's value now, and the way to take it.
 */
export function ServerMovedNote({ value, onTake }: { value: string; onTake: () => void }) {
    const say = useSaveWords();
    return (
        <div data-server-moved="" className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-fg">
            <span className="min-w-0 max-w-[75ch]">{say('moved', { value })}</span>
            <Button type="button" onClick={onTake}>{say('takeServer')}</Button>
        </div>
    );
}
