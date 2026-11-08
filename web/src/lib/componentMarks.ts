import { createLucideIcon, type LucideIcon } from 'lucide-react';

// Component marks lucide has nothing fitting for (alpha.130, owner: "the
// components do not have proper icons"). Drawn by CelikPanel in the line
// style of every other icon (24 grid, 2 px round strokes, outline only).
//
// None of them is a project's logo or mascot, and none imitates one: Python
// is a plain snake (the Python logo is two interlocked snakes in two
// colours), Go is the word's own meaning, a forward arrow (no gopher), Rust
// is a hex nut (the Rust logo is a gear with an R), PowerDNS is a name server
// above the globe, Redis is a key beside its value (no cube), phpPgAdmin is
// a web page with a table. The database engines live in engineIcons.ts.
//
// Lucide'de uygun karşılığı olmayan bileşen işaretleri; CelikPanel'in kendi
// çizimi. Hiçbiri bir projenin logosu ya da maskotu değildir, onu taklit de
// etmez.

/** Python: a snake, coiled in two turns, with its head up. */
export const MarkPython: LucideIcon = createLucideIcon('MarkPython', [
    ['path', { d: 'M15 5H9a3.5 3.5 0 0 0 0 7h6a3.5 3.5 0 0 1 0 7H4', key: 'body' }],
    ['path', { d: 'M15 3h3.5a2 2 0 0 1 0 4H15z', key: 'head' }],
]);

/** Go: go, a forward arrow with speed lines. */
export const MarkGo: LucideIcon = createLucideIcon('MarkGo', [
    ['path', { d: 'M14 6l6 6-6 6', key: 'head' }],
    ['path', { d: 'M20 12H9', key: 'shaft' }],
    ['path', { d: 'M3 12h2', key: 'speed-mid' }],
    ['path', { d: 'M4 7.5h5', key: 'speed-top' }],
    ['path', { d: 'M4 16.5h5', key: 'speed-bottom' }],
]);

/** Rust: a hex nut, the metal fastener of systems work. */
export const MarkRust: LucideIcon = createLucideIcon('MarkRust', [
    ['path', { d: 'M12 2.5l8.25 4.75v9.5L12 21.5l-8.25-4.75v-9.5z', key: 'nut' }],
    ['circle', { cx: '12', cy: '12', r: '3.5', key: 'hole' }],
]);

/** PowerDNS: a name server (a rack unit) above the globe it answers for. */
export const MarkNameServer: LucideIcon = createLucideIcon('MarkNameServer', [
    ['rect', { x: '3', y: '2', width: '18', height: '6', rx: '2', key: 'rack' }],
    ['path', { d: 'M7 5h.01', key: 'light' }],
    ['path', { d: 'M11 5h6', key: 'slot' }],
    ['circle', { cx: '12', cy: '16.5', r: '5', key: 'globe' }],
    ['ellipse', { cx: '12', cy: '16.5', rx: '2', ry: '5', key: 'meridian' }],
    ['path', { d: 'M7 16.5h10', key: 'equator' }],
]);

/** Redis: a key beside the value it holds, a key-value cache. */
export const MarkKeyValue: LucideIcon = createLucideIcon('MarkKeyValue', [
    ['circle', { cx: '6', cy: '12', r: '3', key: 'bow' }],
    ['path', { d: 'M9 12h5', key: 'shaft' }],
    ['path', { d: 'M12 12v2.5', key: 'bit' }],
    ['rect', { x: '15.5', y: '6', width: '6', height: '12', rx: '1.5', key: 'value' }],
    ['path', { d: 'M15.5 12h6', key: 'value-row' }],
]);

/** phpPgAdmin: a web page (a window with its bar) holding a table. */
export const MarkWebTable: LucideIcon = createLucideIcon('MarkWebTable', [
    ['rect', { x: '3', y: '3', width: '18', height: '18', rx: '2', key: 'window' }],
    ['path', { d: 'M3 8h18', key: 'bar' }],
    ['path', { d: 'M6.5 5.5h.01', key: 'dot' }],
    ['path', { d: 'M9.5 5.5h.01', key: 'dot2' }],
    ['path', { d: 'M7 12.5h10', key: 'row' }],
    ['path', { d: 'M7 16.5h10', key: 'row2' }],
]);
