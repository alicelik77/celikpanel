import { Database, createLucideIcon, type LucideIcon } from 'lucide-react';
import './engineMark.css';
import type { DatabaseEngineName } from '../lib/domainDatabases';

// A database engine's mark and colour (alpha.129): CelikPanel's own mark on a
// tile in the engine's colour, light and dark (engineMark.css ships with this
// screen's chunk, not the boot path). MariaDB is never called MySQL: the
// name comes from the server's engine (lib/domainDatabases.ts).
//
// The marks are CelikPanel's, not the projects' logos. The PostgreSQL
// Community Association's trademark policy names only the words as fair use
// and asks for permission for the Slonik logo elsewhere; the MariaDB sea lion
// is MariaDB plc's registered mark with no grant for other products'
// interfaces. So each engine is a database cylinder with its initial, in the
// line style of every other icon. They live here, beside the only screen that
// draws them, and not in lib/componentIcons.tsx: a module shared with the
// Components page becomes a chunk of its own, and every chunk the entry can
// reach lengthens the boot path (measured: +0.17 KiB).
//
// Veritabanı motorunun işareti ve rengi; projelerin logoları değil (marka
// kuralları izin vermiyor), baş harfli veritabanı silindiri.

const cylinder: Parameters<typeof createLucideIcon>[1] = [
    ['ellipse', { cx: '12', cy: '5', rx: '8', ry: '3', key: 'top' }],
    ['path', { d: 'M4 5v14a8 3 0 0 0 16 0V5', key: 'body' }],
];
export const EngineMariaDB: LucideIcon = createLucideIcon('EngineMariaDB', [
    ...cylinder,
    ['path', { d: 'M8.5 18v-6l3.5 4 3.5-4v6', key: 'initial' }],
]);
export const EnginePostgreSQL: LucideIcon = createLucideIcon('EnginePostgreSQL', [
    ...cylinder,
    ['path', { d: 'M10 18v-7h2.75a2 2 0 0 1 0 4H10', key: 'initial' }],
]);

/** The mark of a database engine (mariadb, mysql or postgresql). */
export function engineIcon(engine: DatabaseEngineName): LucideIcon {
    if (engine === 'postgresql') return EnginePostgreSQL;
    if (engine === 'mariadb') return EngineMariaDB;
    return Database;
}

export function EngineMark({ engine, size = 'md' }: { engine: DatabaseEngineName; size?: 'sm' | 'md' }) {
    const Icon = engineIcon(engine);
    const box = size === 'sm' ? 'h-8 w-8 rounded-lg' : 'h-10 w-10 rounded-xl';
    return (
        <span className={`engine-mark inline-flex shrink-0 items-center justify-center ${box}`} data-engine={engine} aria-hidden="true">
            <Icon className={size === 'sm' ? 'h-4 w-4' : 'h-5 w-5'} strokeWidth={2} />
        </span>
    );
}
