import { createContext, useContext, useEffect, useRef } from 'react';
import { useI18n } from '../../../i18n';
import { confirmAction } from '../../../lib/confirmAction';
import { setLeaveAsker } from './leaveAsker';
import { saveWord } from './words';

// Leaving unsaved changes asks first (alpha.132; owner, 8 Nov 2026: settings
// are saved only with Save, and changes that were not saved are not dropped
// unseen). Every screen part with unsaved changes registers here
// (useLeaveGuard; useDraft does it by itself). While one has any:
//
//   - a router navigation and the browser's Back/Forward ask in the panel's
//     own dialog: "Leave without saving" / "Stay on this page" (router.tsx,
//     through ./leaveAsker.ts);
//   - a switch the router does not see (a section of the domain page) asks
//     the same through confirmLeaveIfDirty();
//   - closing or reloading the tab gets the browser's own leave question: the
//     one browser dialogue that cannot be replaced (`beforeunload`), set here
//     and nowhere else.
//
// A running operation's hard blocker (router.tsx useNavigationBlocker) still
// refuses without asking; it is checked first. "Leave" releases every
// registered draft, so the navigation that follows is not asked again.
//
// Kaydedilmemiş değişiklikten çıkmadan önce panelin kendi penceresi sorar:
// "Kaydetmeden çık" / "Sayfada kal". Yönlendiricinin görmediği geçişler
// confirmLeaveIfDirty() ile aynı soruyu sorar; sekmeyi kapatmak tarayıcının tek
// değiştirilemeyen sorusunu (beforeunload) yalnız burada alır.

type Unsaved = number | 'some';

/**
 * A part of a page that can be put away on its own (a closable part of a
 * domain section, alpha.132): the drafts mounted inside it belong to it, so
 * closing it asks only about them and gives up only them.
 * Kendi başına kapatılabilen sayfa parçası; içindeki taslaklar ona aittir.
 */
export const LeaveScope = createContext<object | null>(null);

const registered = new Map<object, Unsaved>();
const scopes = new Map<object, object | null>();
let locale: string | undefined;

const inScope = (scope?: object) => [...registered.keys()].filter((key) => scope === undefined || scopes.get(key) === scope);
const open = (scope?: object) => inScope(scope).map((key) => registered.get(key) ?? 0).filter((count) => count !== 0);

function onBeforeUnload(event: BeforeUnloadEvent) {
    if (open().length === 0) return;
    event.preventDefault();
    event.returnValue = '';
}

let listening = false;

function sync() {
    const unsaved = open().length > 0;
    setLeaveAsker(unsaved ? ask : null);
    if (unsaved === listening || typeof window === 'undefined' || typeof window.addEventListener !== 'function') return;
    listening = unsaved;
    if (unsaved) window.addEventListener('beforeunload', onBeforeUnload);
    else window.removeEventListener('beforeunload', onBeforeUnload);
}

function ask(scope?: object): Promise<boolean> | null {
    const unsaved = open(scope);
    if (unsaved.length === 0) return null;
    const n = unsaved.reduce<Unsaved>((sum, count) => (sum === 'some' || count === 'some' ? 'some' : sum + count), 0);
    const lost = n === 'some' ? saveWord(locale, 'leaveLostSome') : n === 1 ? saveWord(locale, 'leaveLostOne') : saveWord(locale, 'leaveLost', { n });
    // One question at a time: while another question is open the answer is
    // "stay" (lib/confirmAction.ts).
    return confirmAction({
        title: saveWord(locale, 'leaveTitle'),
        tone: 'danger',
        lost: [lost],
        kept: [saveWord(locale, 'leaveKept')],
        confirmLabel: saveWord(locale, 'leave'),
        cancelLabel: saveWord(locale, 'stay'),
    }).then((leave) => {
        if (leave) {
            for (const key of inScope(scope)) registered.set(key, 0);
            sync();
        }
        return leave;
    });
}

/**
 * Asks before a switch the router does not see. Resolves true when nothing is
 * unsaved or the person chose to leave (the changes are then given up), false
 * when they stay. With a scope (LeaveScope), only that part's changes count.
 */
export function confirmLeaveIfDirty(scope?: object): Promise<boolean> {
    return ask(scope) ?? Promise.resolve(true);
}

/**
 * Registers this part's unsaved changes while it is mounted: a count, or
 * 'some' when they cannot be counted (a text that differs). 0: nothing.
 */
export function useLeaveGuard(unsaved: Unsaved): void {
    const { locale: current } = useI18n();
    const key = useRef({}).current;
    const scope = useContext(LeaveScope);
    locale = current;
    useEffect(() => {
        registered.set(key, unsaved);
        scopes.set(key, scope);
        sync();
    }, [key, unsaved, scope]);
    useEffect(() => () => {
        registered.delete(key);
        scopes.delete(key);
        sync();
    }, [key]);
}
