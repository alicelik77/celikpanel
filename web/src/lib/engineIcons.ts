import { createLucideIcon, type LucideIcon } from 'lucide-react';

// The database engines' marks (alpha.130, owner's request): an elephant for
// PostgreSQL and a sea lion for MariaDB, drawn by CelikPanel in the line style
// of every other icon (24 grid, 2 px round strokes).
//
// They are not the projects' logos. The PostgreSQL Community Association's
// trademark policy asks for permission for the Slonik logo outside the uses
// it lists, and the MariaDB sea lion is a registered mark of the MariaDB
// company. So the animals are our own drawings and deliberately unlike the
// logos: a whole elephant in profile (Slonik is a head seen from the front)
// and a resting sea lion facing right (MariaDB's leaps to the left), in
// outline only, in the panel's engine colours (engineMark.css), never the
// logos' colours or shapes.
//
// One module for the Domain's database cards (EngineMark.tsx) and the
// Components page (componentIcons.tsx), so the two screens show the same mark.

export const EnginePostgreSQL: LucideIcon = createLucideIcon('EnginePostgreSQL', [
    ['path', { d: 'M20 19v-8a6 6 0 0 0-6-6H8.5A4.5 4.5 0 0 0 4 9.5v8a1.5 1.5 0 0 0 3 0V14h2v5', key: 'body' }],
    ['path', { d: 'M12 19v-4h5v4', key: 'legs' }],
    ['path', { d: 'M10.5 5.5c1.5 1 2 4.5-1 6.5', key: 'ear' }],
    ['path', { d: 'M6.5 8.5h.01', key: 'eye' }],
]);

export const EngineMariaDB: LucideIcon = createLucideIcon('EngineMariaDB', [
    ['path', { d: 'M6 17h9.5c1.8-1.8 2.6-5.3 2.6-9.3h1.6c.8 0 1.3-.5 1.3-1.2A2.5 2.5 0 0 0 18.5 4c-2.5 0-3.3 2.5-4.8 4.5C11.5 11.6 7.5 13.5 3 13.5l2.5 2-3 2.5z', key: 'body' }],
    ['path', { d: 'M13.5 17l2 3h3', key: 'flipper' }],
    ['path', { d: 'M18.8 5.9h.01', key: 'eye' }],
]);
