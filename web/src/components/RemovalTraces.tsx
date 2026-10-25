// What the system's own traces show using a component about to be removed
// (v0.1.0-alpha.108, D-038 §13): the owner's web-server lines that point at
// its socket or port, services of theirs that depend on it, local programs
// connected to it now. CelikPanel did not set these up, so it cannot move
// them: the dialog names each one, says that it stops working with the
// removal, and that the server owner checks it first. A warning, never a
// refusal. When nothing is found it says so in D-038's words; when a source
// could not be read it says which.
// Sistemin kendi izleri: CelikPanel dışında bileşeni kullananlar, adlarıyla.
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { KeepWords } from './ui';

export interface RemovalTrace {
    kind: 'web_config' | 'unit' | 'process';
    path?: string;
    line?: number;
    text?: string;
    unit?: string;
    relation?: string;
    process?: string;
    pid?: number;
    endpoint?: string;
}

export interface RemovalTraces {
    traces: RemovalTrace[];
    traces_state?: 'checked' | 'partial';
    traces_unread?: string[];
}

const KINDS: ReadonlySet<string> = new Set(['web_config', 'unit', 'process']);
const SOURCES: ReadonlySet<string> = new Set(['web_config', 'units', 'connections']);

/** The traces of a simulation answer; anything malformed is not shown. */
export function decodeRemovalTraces(value: Record<string, unknown>): RemovalTraces {
    const out: RemovalTraces = { traces: [] };
    if (Array.isArray(value.traces)) {
        for (const item of value.traces) {
            const trace = item as Record<string, unknown> | null;
            if (!trace || typeof trace !== 'object' || typeof trace.kind !== 'string' || !KINDS.has(trace.kind)) continue;
            const text = (key: string) => (typeof trace[key] === 'string' ? (trace[key] as string) : undefined);
            const number = (key: string) => (typeof trace[key] === 'number' && Number.isInteger(trace[key]) ? (trace[key] as number) : undefined);
            out.traces.push({
                kind: trace.kind as RemovalTrace['kind'],
                path: text('path'), line: number('line'), text: text('text'), unit: text('unit'), relation: text('relation'),
                process: text('process'), pid: number('pid'), endpoint: text('endpoint'),
            });
        }
    }
    if (value.traces_state === 'checked' || value.traces_state === 'partial') out.traces_state = value.traces_state;
    if (Array.isArray(value.traces_unread)) out.traces_unread = value.traces_unread.filter((source): source is string => typeof source === 'string' && SOURCES.has(source));
    return out;
}

export function removalTraceSentence(t: (key: TranslationKey, vars?: Record<string, string | number>) => string, trace: RemovalTrace): string {
    switch (trace.kind) {
        case 'web_config':
            return t('services.removal.traces.webConfig', { path: trace.path ?? '', line: trace.line ?? 0, endpoint: trace.endpoint ?? '' });
        case 'unit':
            return t('services.removal.traces.unit', { unit: trace.unit ?? '', relation: trace.relation ?? '' });
        default:
            return t('services.removal.traces.process', { process: trace.process ?? '', pid: trace.pid ?? 0, endpoint: trace.endpoint ?? '' });
    }
}

export function RemovalTracesNote({ traces, name }: { traces: RemovalTraces | null | undefined; name: string }) {
    const { t } = useI18n();
    if (!traces || !traces.traces_state) return null;
    const unread = (traces.traces_unread ?? []).map((source) => t(`services.removal.traces.source.${source}` as TranslationKey)).join(', ');
    if (traces.traces.length === 0) {
        return (
            <p className="text-fg-muted" data-removal-traces="none">
                {t('services.removal.traces.none', { name })}
                {traces.traces_state === 'partial' && <> {t('services.removal.traces.partial', { sources: unread })}</>}
            </p>
        );
    }
    return (
        <div className="rounded-lg border border-warning-mark/50 bg-warning-mark/20 px-3 py-2 text-fg" data-removal-traces="found">
            <p className="font-semibold">{t('services.removal.traces.title', { name })}</p>
            <ul className="mt-1 list-disc space-y-1 pl-5">
                {traces.traces.map((trace, index) => (
                    <li key={`${trace.kind}-${index}`} data-removal-trace={trace.kind} className="break-words">
                        <KeepWords text={removalTraceSentence(t, trace)} />
                        {trace.kind === 'web_config' && trace.text && (
                            <code className="mt-0.5 block break-all rounded bg-surface px-1.5 py-0.5 font-mono text-xs text-fg">{trace.text}</code>
                        )}
                    </li>
                ))}
            </ul>
            <p className="mt-1.5">{t('services.removal.traces.next', { name })}</p>
            {traces.traces_state === 'partial' && <p className="mt-1 text-fg-muted">{t('services.removal.traces.partial', { sources: unread })}</p>}
        </div>
    );
}
