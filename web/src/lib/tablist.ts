// What every row of section tabs does the same way (Settings, a domain's
// page; alpha.132): the arrow keys, Home and End move between the tabs, and
// the active tab is scrolled into view inside its own row, which scrolls
// sideways on a narrow screen. Only the row moves, never the page: a tab
// chosen from further down the page does not pull the page up to the row.
// Bölüm sekmesi satırlarının ortak davranışı: ok tuşları, Home ve End
// sekmeler arasında gezer; etkin sekme yalnız kendi satırında görünür kılınır,
// sayfa kaydırılmaz.

const STEPS: Record<string, number> = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 };

/** The tab a key moves to from `index` among `count`, or null for a key that does not move. */
export function nextTabIndex(key: string, index: number, count: number): number | null {
    if (count <= 0) return null;
    if (key === 'Home') return 0;
    if (key === 'End') return count - 1;
    if (!(key in STEPS)) return null;
    return (index + STEPS[key] + count) % count;
}

/** Scrolls the tab's row sideways until the tab is fully in view; the page stays where it is. */
export function revealTabInRow(tab: HTMLElement | null | undefined): void {
    const row = tab?.parentElement;
    if (!tab || !row || row.scrollWidth <= row.clientWidth) return;
    const tabBox = tab.getBoundingClientRect();
    const rowBox = row.getBoundingClientRect();
    const margin = 16;
    if (tabBox.left < rowBox.left) row.scrollLeft -= rowBox.left - tabBox.left + margin;
    else if (tabBox.right > rowBox.right) row.scrollLeft += tabBox.right - rowBox.right + margin;
}
