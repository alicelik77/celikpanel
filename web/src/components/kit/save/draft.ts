import { useCallback, useEffect, useRef, useState } from 'react';
import { useLeaveGuard } from './leaveGuard';
import { useSaveRun, type SaveOutcome } from './saveRun';

// The unsaved values of a settings form over what the server last said
// (alpha.132; owner, 8 Nov 2026: nothing is saved by clicking a setting; a
// changed setting is marked, a bar says how many are not saved, Save or
// Discard). One rule for every settings screen:
//
//   - `known` is the server's last answer (the screen passes the last value it
//     really read, also while a newer read failed). A new answer is a new
//     object; that is how the draft sees the server change underneath it.
//   - An edit is kept with the server value it was made over. A field the
//     person did not touch follows the server. A field the person changed
//     keeps the person's value when the server changes too: it is flagged
//     `moved`, and the screen shows the server's value beside it and offers to
//     take it (`take`). Neither side is overwritten unseen.
//   - An edit the server now holds is no change any more and is dropped.
//   - Save sends only the changed fields. Saved: the draft shows what was sent
//     until the next answer arrives. Refused: the edits stay. Unknown (no
//     answer): the edits stay, and Save stays off until the server has been
//     read again, so nothing is sent a second time blind (D-024).
//   - While anything is unsaved the leave guard asks before the page goes.
//
// Bir ayar formunun sunucunun son söylediği üzerindeki kaydedilmemiş değerleri.
// Dokunulmayan alan sunucuyu izler; değiştirilen alan, sunucu da değişirse
// kişinin değerini korur ve işaretlenir; hiçbir taraf görünmeden ezilmez.
// Kayıt yalnız değişen alanları gönderir; sonucu bilinmeyen kayıttan sonra
// sunucu yeniden okunana dek Kaydet kapalıdır.

interface Edit {
    value: unknown;
    /** The server's value when the field was first changed. */
    from: unknown;
}

type Edits<T> = Partial<Record<keyof T, Edit>>;

const same = (a: unknown, b: unknown) =>
    Object.is(a, b) || (typeof a === 'object' && a !== null && typeof b === 'object' && b !== null && JSON.stringify(a) === JSON.stringify(b));

// The edits whose value differs from `values` (the server's, or what was sent).
function differing<T>(edits: Edits<T>, values: Partial<T>): Edits<T> {
    const kept = Object.fromEntries(Object.entries(edits).filter(([field, edit]) => !same((edit as Edit).value, values[field as keyof T])));
    return Object.keys(kept).length === Object.keys(edits).length ? edits : (kept as Edits<T>);
}

export interface Draft<T extends object> {
    /** What the form shows: the server's values with the person's edits over them; null while nothing is known. */
    values: T | null;
    /** The server's values the draft is measured against; null while nothing is known. */
    base: T | null;
    /** Changes one field. Setting it back to the server's value is no change. */
    set: <K extends keyof T>(field: K, value: T[K]) => void;
    /** The changed fields, in the order of the server's keys. */
    changed: (keyof T)[];
    count: number;
    isChanged: (field: keyof T) => boolean;
    /** The person changed this field and the server changed it too since. */
    isMoved: (field: keyof T) => boolean;
    /** Gives up the edit of one field: the server's value shows again. */
    take: (field: keyof T) => void;
    /** Discard: gives up every edit. */
    reset: () => void;
    saving: boolean;
    outcome: SaveOutcome | null;
    /** The last save's answer was lost and the server has not been read again: Save stays off. */
    held: boolean;
    /**
     * Sends the changes. `send` gets the changed fields only and the whole
     * draft, sends them and says how it ended. A call while a save runs, with
     * nothing changed or while `held`, sends nothing and answers null.
     */
    save: (send: (changes: Partial<T>, values: T) => Promise<SaveOutcome>) => Promise<SaveOutcome | null>;
}

export function useDraft<T extends object>(known: T | null | undefined): Draft<T> {
    const [edits, setEdits] = useState<Edits<T>>({});
    // What an accepted save sent, shown over the answer it was made on until
    // the next answer arrives.
    const [adopted, setAdopted] = useState<{ over: T; values: Partial<T> } | null>(null);
    // The answer a save whose result was lost was made on.
    const [lostOver, setLostOver] = useState<T | null>(null);
    const runner = useSaveRun();

    const base: T | null = known ? (adopted && adopted.over === known ? { ...known, ...adopted.values } : known) : null;
    const baseRef = useRef(base);
    baseRef.current = base;
    const isChanged = (field: keyof T) => {
        const edit = edits[field];
        return !!base && !!edit && !same(edit.value, base[field]);
    };
    const changed = (base ? (Object.keys(base) as (keyof T)[]) : []).filter(isChanged);
    const values = base ? ({ ...base, ...Object.fromEntries(changed.map((field) => [field, edits[field]!.value])) } as T) : null;
    const held = runner.outcome === 'unknown' && lostOver !== null && lostOver === known;

    // A new answer: an edit the server now holds is no change any more.
    useEffect(() => {
        if (baseRef.current) {
            const server = baseRef.current;
            setEdits((current) => differing(current, server));
        }
    }, [known, adopted]);

    // Nothing unsaved any more: a lost answer has nothing left to tell.
    const { outcome, clear } = runner;
    useEffect(() => {
        if (changed.length === 0 && outcome === 'unknown') clear();
    }, [changed.length, outcome, clear]);

    useLeaveGuard(changed.length);

    const set = useCallback(<K extends keyof T>(field: K, value: T[K]) => {
        const server = baseRef.current;
        if (!server) return;
        setEdits((current) => {
            if (same(value, server[field])) {
                if (!current[field]) return current;
                const next = { ...current };
                delete next[field];
                return next;
            }
            return { ...current, [field]: { value, from: current[field] ? current[field]!.from : server[field] } };
        });
    }, []);

    const take = useCallback((field: keyof T) => {
        setEdits((current) => {
            if (!current[field]) return current;
            const next = { ...current };
            delete next[field];
            return next;
        });
    }, []);

    const reset = useCallback(() => setEdits({}), []);

    const save = async (send: (changes: Partial<T>, values: T) => Promise<SaveOutcome>) => {
        if (!values || !known || changed.length === 0 || held) return null;
        const over = known;
        const sent = Object.fromEntries(changed.map((field) => [field, edits[field]!.value])) as Partial<T>;
        const result = await runner.run(() => send(sent, values));
        if (result === 'saved') {
            setAdopted({ over, values: sent });
            // What was sent is what the server holds now.
            setEdits((current) => differing(current, sent));
        } else if (result === 'unknown') {
            // Held until an answer newer than the one the save was made on.
            setLostOver(over);
        }
        return result;
    };

    return {
        values,
        base,
        set,
        changed,
        count: changed.length,
        isChanged,
        isMoved: (field) => isChanged(field) && !same(edits[field]!.from, base![field]),
        take,
        reset,
        saving: runner.saving,
        outcome: runner.outcome,
        held,
        save,
    };
}
