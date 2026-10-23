import { useEffect, useState } from 'react';
import { Code2 } from 'lucide-react';
import { useI18n } from '../i18n';
import { Button, Checking, CouldNotCheck, KeepWords, Spinner } from './ui';
import { componentName } from '../lib/componentName';
import { movePHPTool, phpToolCauseText, readPHPTools, type PHPToolName, type PHPToolsInfo } from '../lib/phpTools';

// Which PHP version webmail (Roundcube), phpMyAdmin or phpPgAdmin runs on, in
// the tool's own Manage window (alpha.104, owner decision of 2026-10-23),
// like a site's PHP version in its settings. Only installed PHP-FPM versions
// are offered. Choosing one moves the tool there through the Panel: its PHP
// extensions are installed for that version when missing, the web server's
// configuration is checked and reloaded, and the tool must answer as itself;
// otherwise it is put back where it was, and the card says so.
//
// Bir aracın koştuğu PHP sürümü, aracın kendi Yönet penceresinde; yalnız kurulu
// sürümler sunulur. Seçim doğrulanır; tutmazsa araç eski sürümüne döner.

type MoveResult =
    | { kind: 'done'; version: string }
    | { kind: 'restored'; version: string; previous: string; reason: string }
    | { kind: 'failed'; version: string; reason: string };

export function PHPToolVersion({ serviceId, tool }: { serviceId: string; tool: PHPToolName }) {
    const { t } = useI18n();
    const name = componentName({ id: serviceId, name: serviceId }, t);
    const [info, setInfo] = useState<PHPToolsInfo | null>(null);
    const [token, setToken] = useState(0);
    const [choice, setChoice] = useState('');
    const [moving, setMoving] = useState(false);
    const [result, setResult] = useState<MoveResult | null>(null);

    useEffect(() => {
        const controller = typeof AbortController === 'undefined' ? null : new AbortController();
        setInfo(null);
        void readPHPTools(controller?.signal).then((next) => { if (!controller?.signal.aborted) setInfo(next); });
        return () => controller?.abort();
    }, [token]);

    const state = info?.state === 'read' ? info.tools.find((item) => item.tool === tool) : undefined;
    const versions = info?.state === 'read' ? info.versions.map((usage) => usage.version) : [];
    const current = state?.version ?? '';
    // The current version is offered again when it is not ready or lacks the
    // tool's extension packages: choosing it again installs and tests them.
    // Hazır değilse ya da eklenti paketleri eksikse mevcut sürüm yeniden sunulur.
    const options = versions.filter((version) => version !== current || !state?.ready || !state.extensions_present);
    // Lacking extension packages on a running version: that version first,
    // as the card's sentence says. / Eklentisi eksikse önce o sürüm.
    const fallback = state?.ready && !state.extensions_present && options.includes(current) ? current : options[0] ?? '';
    const selected = options.includes(choice) ? choice : fallback;

    const move = async () => {
        if (!selected || moving) return;
        setMoving(true);
        setResult(null);
        const answer = await movePHPTool(tool, selected);
        if (answer.kind === 'moved') {
            setResult({ kind: 'done', version: selected });
        } else if (answer.kind === 'lost') {
            // The answer was lost: the move may or may not have happened.
            // Yanıt kayboldu: taşıma olmuş da olabilir, olmamış da.
            setResult({ kind: 'failed', version: selected, reason: t('component.phpTool.answerLost') });
        } else {
            // The cause in the reader's language (alpha.104 review); when the
            // tool could not be put back, the server's own words follow.
            // Neden okurun dilinde; geri konamadıysa sunucunun sözü de eklenir.
            const failure = answer.failure;
            const said = failure?.detail || failure?.error || '';
            const cause = phpToolCauseText(t, failure?.cause ?? null, said);
            const reason = failure && !failure.restored && failure.mutation_applied && failure.error
                ? cause + ' — ' + t('component.phpTool.cause.other', { detail: failure.error })
                : cause;
            setResult(failure?.restored
                ? { kind: 'restored', version: selected, previous: failure.previous || current, reason }
                : { kind: 'failed', version: selected, reason });
        }
        setMoving(false);
        setToken((n) => n + 1);
    };

    return (
        <section className="rounded-2xl border border-border bg-surface" data-php-tool={tool}>
            <header className="flex items-center gap-2 border-b border-border px-5 py-3">
                <Code2 className="h-4 w-4 text-fg-muted" />
                <h2 className="text-sm font-semibold text-fg">{t('component.phpTool.title')}</h2>
            </header>
            <div className="space-y-3 p-5 text-sm">
                {result && (
                    <p className={result.kind === 'done' ? 'text-fg' : 'rounded-lg border border-danger/30 bg-danger/5 p-3 text-fg'} data-php-tool-result={result.kind}>
                        <KeepWords text={result.kind === 'done'
                            ? t('component.phpTool.done', { tool: name, version: result.version })
                            : result.kind === 'restored'
                                ? t('component.phpTool.restored', { tool: name, version: result.version, previous: result.previous, reason: result.reason })
                                : t('component.phpTool.failed', { tool: name, version: result.version, reason: result.reason })} />
                    </p>
                )}
                {info === null ? (
                    <Checking label={t('component.phpTool.checking', { tool: name })} />
                ) : info.state === 'unknown' ? (
                    <CouldNotCheck text={t('component.phpTool.unknown', { tool: name })} onRetry={() => setToken((n) => n + 1)} />
                ) : !state?.installed ? (
                    <p className="text-fg-muted" data-php-tool-state="not-installed">{t('component.phpTool.notInstalled', { tool: name })}</p>
                ) : (
                    <>
                        <p className={state.version && (!state.ready || !state.extensions_present) ? 'font-semibold text-fg' : 'text-fg'} data-php-tool-state={!state.version ? 'none' : state.ready ? (state.extensions_present ? 'ready' : 'extensions') : state.version_installed ? 'stopped' : 'gone'}>
                            <KeepWords text={!state.version
                                ? t('component.phpTool.none', { tool: name })
                                : state.ready
                                    ? (state.extensions_present
                                        ? t('component.phpTool.current', { tool: name, version: state.version })
                                        : t('component.phpTool.missingExtensions', { tool: name, version: state.version }))
                                    : state.version_installed
                                        ? t('component.phpTool.stopped', { tool: name, version: state.version })
                                        : t('component.phpTool.gone', { tool: name, version: state.version })} />
                        </p>
                        {options.length === 0 ? (
                            <p className="text-fg-muted" data-php-tool-options="none">{t('component.phpTool.onlyOne', { tool: name })}</p>
                        ) : (
                            <div className="flex flex-wrap items-center gap-2">
                                <label className="flex items-center gap-2 text-fg">
                                    <span>{t('component.phpTool.choose')}</span>
                                    <select
                                        value={selected}
                                        onChange={(event) => setChoice(event.target.value)}
                                        disabled={moving}
                                        className="rounded-lg border border-border-strong bg-surface px-2 py-1 font-mono text-sm text-fg max-sm:min-h-11"
                                        data-php-tool-select=""
                                    >
                                        {options.map((version) => <option key={version} value={version}>PHP {version}</option>)}
                                    </select>
                                </label>
                                <Button variant="secondary" onClick={() => void move()} disabled={moving || !selected} loading={moving} className="max-sm:min-h-11">
                                    {moving ? t('component.phpTool.moving', { version: selected }) : t('component.phpTool.apply', { version: selected })}
                                </Button>
                            </div>
                        )}
                        {/* A long waiting sentence keeps its spinner whole on a phone.
                            Uzun bekleme cümlesi telefonda dönen işaretini korur. */}
                        {moving && (
                            <div className="flex items-start gap-2 text-sm text-fg-muted" data-php-tool-waiting="">
                                <span className="mt-0.5 shrink-0"><Spinner size="xs" label={t('component.phpTool.movingWait', { tool: name, version: selected })} /></span>
                                <span aria-hidden="true">{t('component.phpTool.movingWait', { tool: name, version: selected })}</span>
                            </div>
                        )}
                        <p className="text-xs text-fg-subtle">{t('component.phpTool.note', { tool: name })}</p>
                    </>
                )}
            </div>
        </section>
    );
}
