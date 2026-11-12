import { useCallback, useEffect, useRef, useState } from 'react';

// The one core of every save a screen sends (alpha.132; D-024), shared by the
// settings drafts (./draft.ts) and the configuration file editors
// (../../../lib/configFile.ts): one save at a time, and how it ended.
//
//   saved     the server accepted it.
//   refused   the server answered and did not take it (the screen says why;
//             what the person entered stays).
//   unknown   no answer arrived, or the answer could not be read: whether it
//             was saved is not known. Nothing is sent a second time.
//
// A second press while a save runs starts nothing: it answers null at once,
// before the busy state has even been drawn.
//
// Bir ekranın gönderdiği her kaydın tek çekirdeği: aynı anda tek kayıt ve
// nasıl bittiği (kaydedildi, reddedildi, bilinmiyor). Kayıt sürerken ikinci
// basış hiçbir şey başlatmaz.

export type SaveOutcome = 'saved' | 'refused' | 'unknown';

export interface SaveRun {
    saving: boolean;
    /** How the last save ended; null before the first and after `clear`. */
    outcome: SaveOutcome | null;
    /** Runs one save. A throw is an unknown result; a call while one runs answers null. */
    run: (send: () => Promise<SaveOutcome>) => Promise<SaveOutcome | null>;
    clear: () => void;
}

export function useSaveRun(): SaveRun {
    const [saving, setSaving] = useState(false);
    const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
    const busy = useRef(false);
    const mounted = useRef(true);
    useEffect(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
        };
    }, []);
    const run = useCallback(async (send: () => Promise<SaveOutcome>) => {
        if (busy.current) return null;
        busy.current = true;
        setSaving(true);
        setOutcome(null);
        let result: SaveOutcome;
        try {
            result = await send();
        } catch {
            result = 'unknown';
        }
        busy.current = false;
        if (mounted.current) {
            setSaving(false);
            setOutcome(result);
        }
        return result;
    }, []);
    const clear = useCallback(() => setOutcome(null), []);
    return { saving, outcome, run, clear };
}
