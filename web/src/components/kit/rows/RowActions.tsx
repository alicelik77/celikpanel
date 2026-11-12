import { useEffect, useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react';
import { createPortal } from 'react-dom';
import { ChevronDown, type LucideIcon } from 'lucide-react';
import { useI18n } from '../../../i18n';
import { Button } from '../../ui';

// A list row's actions as words (alpha.132, principle P3): at most two
// buttons with text for the most frequent actions, then one "More" menu whose
// items are words too. An action that destroys something is last in that
// menu, in red, after a line. Every action carries its word; an icon may
// accompany it, never replace it.
//
// The menu is the product's only one: role="menu", arrow keys, Home/End,
// Escape and Tab close it and give the focus back to its button; it closes on
// a click outside, on scroll and on resize. It is drawn on the page's top
// layer (a portal), so a list that scrolls sideways never clips it, and it
// opens upward when the room below the button is too small.
//
// On a phone the buttons share the row's width and are at least 44 px high;
// a list that turns its rows into cards puts this under the row's facts.
//
// Satırın işlemleri sözcükle: en çok iki yazılı düğme, sonra "Diğer" menüsü.
// Silen işlem menünün sonunda, kırmızı ve çizgiyle ayrılmış. Menü klavye ve
// ekran okuyucuyla kullanılır, liste kabının içinde kesilmez, sayfanın
// altında yukarı açılır.

export interface RowAction {
    /** Stable id: the React key and the data-row-action attribute. */
    id: string;
    /** The action's word, shown on the button or menu item. */
    label: string;
    icon?: LucideIcon;
    /** Runs the action (its promise, if any, is handed back to the caller of the click). Ignored when `href` is set. */
    onSelect?: () => unknown;
    /** An address opened in a new tab (a link, not a script). */
    href?: string;
    disabled?: boolean;
    /** Why it cannot be used now: a menu item shows it under its word, a button as its title. */
    disabledReason?: string;
    /** Destroys something: in the menu it goes last, in red, after a separator. */
    danger?: boolean;
}

// The look of a secondary Button, for the one visible action that is a link.
const linkButtonClass = 'inline-flex items-center gap-1.5 rounded-md border border-border-strong bg-surface px-3 py-1.5 text-sm font-medium text-fg transition-colors hover:bg-surface-2';
// On a phone the visible actions share the width and are touch sized.
const phoneButton = 'whitespace-nowrap max-sm:min-h-11 max-sm:flex-1 max-sm:justify-center';

export function RowActions({ name, actions, more = [], className }: {
    /** What the row is ("kahvedukkani.dr.test"): names the menu for a screen reader. */
    name: string;
    /** The visible buttons, most frequent first; at most two. */
    actions: [] | [RowAction] | [RowAction, RowAction];
    /** The rest, as menu items; dangerous ones are moved to the end. */
    more?: RowAction[];
    className?: string;
}) {
    return (
        <div className={`flex flex-wrap items-center justify-end gap-2 sm:flex-nowrap max-sm:w-full ${className ?? ''}`} data-row-actions={name}>
            {actions.map((action) => <VisibleAction key={action.id} action={action} />)}
            {more.length > 0 && <MoreMenu name={name} items={more} />}
        </div>
    );
}

function VisibleAction({ action }: { action: RowAction }) {
    if (action.href && !action.disabled) {
        return (
            <a href={action.href} target="_blank" rel="noopener noreferrer" className={`${linkButtonClass} ${phoneButton}`} data-row-action={action.id}>
                {action.icon && <action.icon className="h-4 w-4" aria-hidden="true" />}
                {action.label}
            </a>
        );
    }
    return (
        <Button
            type="button"
            variant={action.danger ? 'danger' : 'secondary'}
            icon={action.icon}
            disabled={action.disabled}
            title={action.disabled ? action.disabledReason : undefined}
            onClick={action.onSelect}
            className={phoneButton}
            data-row-action={action.id}
        >
            {action.label}
        </Button>
    );
}

type Opening = false | 'first' | 'last';

function MoreMenu({ name, items }: { name: string; items: RowAction[] }) {
    const { t } = useI18n();
    const id = useId();
    const [open, setOpen] = useState<Opening>(false);
    const [place, setPlace] = useState<{ top: number; left: number } | null>(null);
    const button = useRef<HTMLButtonElement | null>(null);
    const menu = useRef<HTMLDivElement | null>(null);
    const entries = useRef<Array<HTMLElement | null>>([]);
    // Safe actions first, in the screen's order; dangerous ones last.
    const safe = items.filter((item) => !item.danger);
    const danger = items.filter((item) => item.danger);

    const close = (refocus: boolean) => {
        setOpen(false);
        setPlace(null);
        if (refocus) button.current?.focus();
    };

    // Placed under the button, its right edge on the button's, inside the
    // window; above the button when the room below is too small.
    useLayoutEffect(() => {
        if (!open || !button.current || !menu.current) return;
        const anchor = button.current.getBoundingClientRect();
        const width = menu.current.offsetWidth;
        const height = menu.current.offsetHeight;
        const gap = 4;
        const edge = 8;
        const below = anchor.bottom + gap;
        const above = anchor.top - gap - height;
        const top = below + height > window.innerHeight - edge && above >= edge ? above : below;
        const left = Math.min(Math.max(edge, anchor.right - width), Math.max(edge, window.innerWidth - width - edge));
        setPlace({ top, left });
    }, [open]);

    // Focus the first (or last) item once the menu stands in its place.
    useEffect(() => {
        if (!open || !place) return;
        const list = entries.current.filter(Boolean) as HTMLElement[];
        (open === 'last' ? list[list.length - 1] : list[0])?.focus({ preventScroll: true });
        // Runs when the menu has just been placed, not on every opening change.
    }, [place]);

    // A click outside, a scroll of the page or a list, or a resized window
    // closes the menu: it would otherwise float away from its row.
    useEffect(() => {
        if (!open) return;
        const outside = (event: Event) => {
            const target = event.target as Node | null;
            if (target && (menu.current?.contains(target) || button.current?.contains(target))) return;
            close(false);
        };
        const moved = (event: Event) => {
            const target = event.target as Node | null;
            if (target && menu.current?.contains(target)) return;
            close(false);
        };
        const resized = () => close(false);
        // Scroll events do not bubble: caught on the way down, a list's own
        // scroll is seen as well as the page's.
        document.addEventListener('pointerdown', outside, true);
        document.addEventListener('scroll', moved, true);
        window.addEventListener?.('resize', resized);
        return () => {
            document.removeEventListener('pointerdown', outside, true);
            document.removeEventListener('scroll', moved, true);
            window.removeEventListener?.('resize', resized);
        };
    }, [open]);

    const onButtonKey = (event: KeyboardEvent<HTMLButtonElement>) => {
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault();
            setOpen(event.key === 'ArrowDown' ? 'first' : 'last');
        }
    };

    const onMenuKey = (event: KeyboardEvent<HTMLDivElement>) => {
        const list = entries.current.filter(Boolean) as HTMLElement[];
        const at = list.indexOf(document.activeElement as HTMLElement);
        const go = (index: number) => list[(index + list.length) % list.length]?.focus({ preventScroll: true });
        switch (event.key) {
            case 'ArrowDown': event.preventDefault(); go(at + 1); break;
            case 'ArrowUp': event.preventDefault(); go(at < 0 ? list.length - 1 : at - 1); break;
            case 'Home': event.preventDefault(); go(0); break;
            case 'End': event.preventDefault(); go(list.length - 1); break;
            case 'Escape': event.preventDefault(); close(true); break;
            case 'Tab': event.preventDefault(); close(true); break;
        }
    };

    const choose = (item: RowAction) => {
        if (item.disabled) return undefined;
        close(true);
        return item.href ? undefined : item.onSelect?.();
    };

    entries.current = [];
    const entry = (item: RowAction, index: number) => {
        const tone = item.disabled ? 'cursor-not-allowed text-fg-muted' : item.danger ? 'text-danger hover:bg-danger/10 focus:bg-danger/10' : 'text-fg hover:bg-surface-2 focus:bg-surface-2';
        const cls = `flex w-full items-start gap-2 rounded-md px-3 py-2 text-left text-sm focus:outline-none max-sm:min-h-11 ${tone}`;
        const body = (
            <>
                {item.icon && <item.icon className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />}
                <span className="min-w-0">
                    <span className="block">{item.label}</span>
                    {item.disabled && item.disabledReason && <span className="block text-xs text-fg-muted">{item.disabledReason}</span>}
                </span>
            </>
        );
        const ref = (node: HTMLElement | null) => { entries.current[index] = node; };
        if (item.href && !item.disabled) {
            return (
                <a key={item.id} ref={ref} role="menuitem" tabIndex={-1} href={item.href} target="_blank" rel="noopener noreferrer"
                    onClick={() => choose(item)} className={cls} data-row-action={item.id}>
                    {body}
                </a>
            );
        }
        return (
            <button key={item.id} ref={ref} type="button" role="menuitem" tabIndex={-1} aria-disabled={item.disabled || undefined}
                onClick={() => choose(item)} className={cls} data-row-action={item.id}>
                {body}
            </button>
        );
    };

    const menuId = `${id}-menu`;
    return (
        <>
            <button
                ref={button}
                type="button"
                id={`${id}-button`}
                aria-haspopup="menu"
                aria-expanded={open ? true : false}
                aria-controls={open ? menuId : undefined}
                aria-label={t('kit.rows.moreFor', { name })}
                onClick={() => (open ? close(false) : setOpen('first'))}
                onKeyDown={onButtonKey}
                className={`inline-flex items-center gap-1.5 rounded-md border border-border-strong px-3 py-1.5 text-sm font-medium text-fg transition-colors hover:bg-surface-2 ${open ? 'bg-surface-2' : 'bg-surface'} ${phoneButton}`}
                data-row-more={name}
            >
                {t('kit.rows.more')}
                <ChevronDown className="h-4 w-4" aria-hidden="true" />
            </button>
            {open && createPortal(
                <div
                    ref={menu}
                    id={menuId}
                    role="menu"
                    aria-labelledby={`${id}-button`}
                    onKeyDown={onMenuKey}
                    style={{ position: 'fixed', top: place?.top ?? 0, left: place?.left ?? 0, visibility: place ? 'visible' : 'hidden' }}
                    className="z-[70] min-w-[12rem] max-w-[calc(100vw-1rem)] rounded-lg border border-border bg-surface p-1 shadow-lg"
                    data-row-menu={name}
                >
                    {safe.map((item, index) => entry(item, index))}
                    {safe.length > 0 && danger.length > 0 && <div role="separator" className="my-1 border-t border-border" />}
                    {danger.map((item, index) => entry(item, safe.length + index))}
                </div>,
                document.body,
            )}
        </>
    );
}
