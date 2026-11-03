import { useEffect, useRef, type RefObject } from 'react';

// Keeps the keyboard inside an open drawer or tour step: Tab and Shift+Tab
// wrap at the ends, Escape calls onEscape, and the focus goes back to where
// it was when the trap closes.
// Açık çekmecede ya da tur adımında klavyeyi içeride tutar.

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function useFocusTrap(ref: RefObject<HTMLElement>, onEscape: () => void, initial?: RefObject<HTMLElement>): void {
    const escape = useRef(onEscape);
    escape.current = onEscape;
    useEffect(() => {
        const before = document.activeElement as HTMLElement | null;
        const box = ref.current;
        (initial?.current ?? box?.querySelector<HTMLElement>(FOCUSABLE) ?? box)?.focus();
        const onKey = (event: KeyboardEvent) => {
            if (event.key === 'Escape') {
                event.preventDefault();
                escape.current();
                return;
            }
            if (event.key !== 'Tab' || !ref.current) return;
            const items = [...ref.current.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((item) => item.offsetParent !== null || item === document.activeElement);
            if (items.length === 0) {
                event.preventDefault();
                return;
            }
            const first = items[0];
            const last = items[items.length - 1];
            const inside = ref.current.contains(document.activeElement);
            if (event.shiftKey && (document.activeElement === first || !inside)) {
                event.preventDefault();
                last.focus();
            } else if (!event.shiftKey && (document.activeElement === last || !inside)) {
                event.preventDefault();
                first.focus();
            }
        };
        document.addEventListener('keydown', onKey);
        return () => {
            document.removeEventListener('keydown', onKey);
            if (before && before.isConnected) before.focus();
        };
        // The trap is set once per opening; the refs are stable.
    }, []);
}
