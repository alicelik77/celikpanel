import { Database, type LucideIcon } from 'lucide-react';
import './engineMark.css';
import type { DatabaseEngineName } from '../lib/domainDatabases';
import { EngineMariaDB, EnginePostgreSQL } from '../lib/engineIcons';

// A database engine's mark and colour (alpha.129): CelikPanel's own mark on a
// tile in the engine's colour, light and dark (engineMark.css ships with this
// screen's chunk, not the boot path). MariaDB is never called MySQL: the
// name comes from the server's engine (lib/domainDatabases.ts). The marks
// themselves (alpha.130: an elephant and a sea lion of our own drawing, not
// the projects' logos) live in lib/engineIcons.ts, shared with the
// Components page.

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
