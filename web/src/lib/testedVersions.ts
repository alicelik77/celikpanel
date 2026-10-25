// "Tested with this release" or "installable, not tested" (v0.1.0-alpha.108,
// D-038 §15): the support table's answer says which version lines this
// CelikPanel release was tested with on this server's distribution release
// (internal/supportdata/tested-versions.json, replaced with each release's
// real-system evidence). Information, never a block; nothing is said when the
// server does not report it.
// "Bu sürümle denendi" ya da "kurulabilir, denenmedi": yalnız bilgi.
import { supportLine, type SupportComponent, type SupportTable } from './supportTable';

export type TestedMarkKind = 'tested' | 'not_tested';

export function testedMark(table: SupportTable | null, component: SupportComponent, version: string): TestedMarkKind | null {
    if (!table?.tested_release || !table.tested) return null;
    const line = supportLine(component, version);
    if (!line) return null;
    return (table.tested[component] ?? []).includes(line) ? 'tested' : 'not_tested';
}
