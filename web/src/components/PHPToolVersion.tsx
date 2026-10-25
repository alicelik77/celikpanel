import { useEffect, useRef, useState } from 'react';
import { Code2 } from 'lucide-react';
import { useI18n } from '../i18n';
import { Button, Checking, CouldNotCheck, KeepWords, Spinner } from './ui';
import { componentName } from '../lib/componentName';
import { movePHPTool, phpToolCauseText, provePHPTool, readPHPTools, type PHPToolName, type PHPToolsInfo } from '../lib/phpTools';

// Which PHP version webmail (Roundcube), phpMyAdmin or phpPgAdmin runs on, in
// the tool's own Manage window (alpha.104, owner decision of 2026-10-23),
// like a site's PHP version in its settings. Only installed PHP-FPM versions
// are offered. Choosing one moves the tool there through the Panel: its PHP
// extensions are installed for that version when missing, the web server's
// configuration is checked and reloaded, and the tool must answer as itself;
// otherwise it is put back where it was, and the card says so.
//
// alpha.108 (Boston): "not running" is said only when the version's socket is
// missing or its unit is stopped or failed; a failed proof says its own cause
// and offers Check again, which asks the tool again and moves nothing. When
// an answer is lost the card reads the state (bounded, visibly) and never
// sends the change again.
//
// Bir aracın koştuğu PHP sürümü, aracın kendi Yönet penceresinde; yalnız kurulu
// sürümler sunulur. Seçim doğrulanır; tutmazsa araç eski sürümüne döner.
// "Çalışmıyor" yalnız soket yoksa ya da birim durmuşsa söylenir; başarısız
// kanıt kendi nedenini söyler. Yanıt kaybolursa kart durumu okur, yeniden göndermez.

type MoveResult =
    | { kind: 'done'; version: string }
    | { kind: 'restored'; version: string; previous: string; reason: string }
    | { kind: 'failed'; version: string; reason: string }
    | { kind: 'proofOnly'; version: string; reason: string }
    | { kind: 'checked'; version: string }
    | { kind: 'checkFailed'; version: string; reason: string }
    | { kind: 'checkBusy' }
    | { kind: 'checkRefused'; reason: string }
    | { kind: 'reconciling'; lost: 'move' | 'check'; attempt: number; busy: boolean }
    | { kind: 'reconciledMoved'; version: string }
    | { kind: 'reconciledRead'; current: string }
    | { kind: 'reconcileGaveUp' };

// The bounded read-back after a lost answer: up to 24 reads, 5 s apart (two
// minutes, beyond a move's own worst case of about a minute and a half).
// Kaybolan yanıttan sonra sınırlı geri okuma: 5 sn arayla en çok 24 okuma.
const RECONCILE_READS = 24;
const STOPPED_UNIT = new Set(['inactive', 'failed']);

export function PHPToolVersion({ serviceId, tool, reconcileDelayMs = 5000 }: { serviceId: string; tool: PHPToolName; reconcileDelayMs?: number }) {
    const { t } = useI18n();
    const name = componentName({ id: serviceId, name: serviceId }, t);
    const [info, setInfo] = useState<PHPToolsInfo | null>(null);
    const [token, setToken] = useState(0);
    const [choice, setChoice] = useState('');
    const [moving, setMoving] = useState(false);
    const [checking, setChecking] = useState(false);
    const [result, setResult] = useState<MoveResult | null>(null);
    const alive = useRef(true);
    useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);

    useEffect(() => {
        const controller = typeof AbortController === 'undefined' ? null : new AbortController();
        setInfo(null);
        void readPHPTools(controller?.signal).then((next) => { if (!controller?.signal.aborted) setInfo(next); });
        return () => controller?.abort();
    }, [token]);

    const state = info?.state === 'read' ? info.tools.find((item) => item.tool === tool) : undefined;
    const versions = info?.state === 'read' ? info.versions.map((usage) => usage.version) : [];
    const current = state?.version ?? '';
    // Not running: the socket is missing or the unit says stopped/failed,
    // read now. A failed proof alone is never "not running" (Boston).
    // Çalışmıyor: soket yok ya da birim durmuş; yalnız başarısız kanıt değil.
    const stopped = !!state?.version_installed && (!state.socket_ready || STOPPED_UNIT.has(state.fpm_state));
    const proofFailed = !!state?.proof_failed && !!state.version_installed && !stopped;
    // The current version is offered again when it is not ready or lacks the
    // tool's extension packages: choosing it again installs and tests them. A
    // failed proof alone is answered by Check again instead.
    // Hazır değilse ya da eklenti paketleri eksikse mevcut sürüm yeniden sunulur.
    const offerCurrent = !state?.extensions_present || (!state?.ready && !proofFailed);
    const options = versions.filter((version) => version !== current || offerCurrent);
    // Lacking extension packages on a running version: that version first,
    // as the card's sentence says. / Eklentisi eksikse önce o sürüm.
    const fallback = state?.ready && !state.extensions_present && options.includes(current) ? current : options[0] ?? '';
    const selected = options.includes(choice) ? choice : fallback;
    const busy = moving || checking || result?.kind === 'reconciling';

    // After a lost answer: read the state until it is known, waiting while
    // the server still runs a change to this tool. Never sends the change again.
    // Kaybolan yanıttan sonra durum bilinene dek okunur; değişiklik yeniden gönderilmez.
    const reconcile = async (lost: 'move' | 'check', target: string) => {
        let waiting = false;
        for (let attempt = 1; attempt <= RECONCILE_READS; attempt++) {
            if (!alive.current) return;
            setResult({ kind: 'reconciling', lost, attempt, busy: waiting });
            const next = await readPHPTools();
            if (!alive.current) return;
            if (next.state === 'read') {
                setInfo(next);
                const change = next.changing;
                waiting = !!change && (change.kind === 'other' || change.tool === tool);
                if (!waiting) {
                    const read = next.tools.find((item) => item.tool === tool);
                    setResult(lost === 'move' && read && read.version === target && read.ready
                        ? { kind: 'reconciledMoved', version: target }
                        : { kind: 'reconciledRead', current: read?.version ?? '' });
                    return;
                }
            }
            await new Promise((resolve) => setTimeout(resolve, reconcileDelayMs));
        }
        if (alive.current) setResult({ kind: 'reconcileGaveUp' });
    };

    const move = async () => {
        if (!selected || busy) return;
        setMoving(true);
        setResult(null);
        const answer = await movePHPTool(tool, selected);
        if (answer.kind === 'lost') {
            // The answer was lost: the move may or may not have happened.
            // Read, never resend. / Yanıt kayboldu: okunur, yeniden gönderilmez.
            setMoving(false);
            await reconcile('move', selected);
            return;
        }
        if (answer.kind === 'moved') {
            setResult({ kind: 'done', version: selected });
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
            setResult(failure?.proof_only
                ? { kind: 'proofOnly', version: selected, reason }
                : failure?.restored
                    ? { kind: 'restored', version: selected, previous: failure.previous || current, reason }
                    : { kind: 'failed', version: selected, reason });
        }
        setMoving(false);
        setToken((n) => n + 1);
    };

    // Check again (alpha.108): asks the tool on the version it runs on.
    // Yeniden denetle: aracı çalıştığı sürümde sorar.
    const check = async () => {
        if (busy || !current) return;
        setChecking(true);
        setResult(null);
        const answer = await provePHPTool(tool);
        setChecking(false);
        if (answer.kind === 'lost') {
            await reconcile('check', current);
            return;
        }
        if (answer.kind === 'proved') setResult({ kind: 'checked', version: answer.version || current });
        else if (answer.kind === 'failed') setResult({ kind: 'checkFailed', version: answer.version || current, reason: phpToolCauseText(t, answer.cause, '') });
        else if (answer.kind === 'busy') setResult({ kind: 'checkBusy' });
        else setResult({ kind: 'checkRefused', reason: phpToolCauseText(t, answer.failure?.cause ?? null, answer.failure?.error || '') });
        setToken((n) => n + 1);
    };

    const resultText = (r: MoveResult): string => {
        switch (r.kind) {
            case 'done': return t('component.phpTool.done', { tool: name, version: r.version });
            case 'restored': return t('component.phpTool.restored', { tool: name, version: r.version, previous: r.previous, reason: r.reason });
            case 'failed': return t('component.phpTool.failed', { tool: name, version: r.version, reason: r.reason });
            case 'proofOnly': return t('component.phpTool.proofOnlyFailed', { tool: name, version: r.version, reason: r.reason });
            case 'checked': return t('component.phpTool.checkPassed', { tool: name, version: r.version });
            case 'checkFailed': return t('component.phpTool.checkFailed', { tool: name, version: r.version, reason: r.reason });
            case 'checkBusy': return t('component.phpTool.checkBusy');
            case 'checkRefused': return t('component.phpTool.checkRefused', { reason: r.reason });
            case 'reconciling': return (r.lost === 'move'
                ? t('component.phpTool.failed', { tool: name, version: selected || current, reason: t('component.phpTool.answerLost') })
                : t('component.phpTool.checkLost')) + ' ' + (r.busy
                ? t('component.phpTool.reconcileBusy')
                : t('component.phpTool.reconciling', { attempt: r.attempt, total: RECONCILE_READS }));
            case 'reconciledMoved': return t('component.phpTool.reconciledMoved', { tool: name, version: r.version });
            case 'reconciledRead': return t('component.phpTool.reconciledRead', { tool: name, current: r.current || '—' });
            case 'reconcileGaveUp': return t('component.phpTool.reconcileGaveUp', { seconds: Math.round((RECONCILE_READS * reconcileDelayMs) / 1000) });
        }
    };
    const calm = result && (result.kind === 'done' || result.kind === 'checked' || result.kind === 'reconciledMoved' || result.kind === 'reconciling' || result.kind === 'reconciledRead');

    return (
        <section className="rounded-2xl border border-border bg-surface" data-php-tool={tool}>
            <header className="flex items-center gap-2 border-b border-border px-5 py-3">
                <Code2 className="h-4 w-4 text-fg-muted" />
                <h2 className="text-sm font-semibold text-fg">{t('component.phpTool.title')}</h2>
            </header>
            <div className="space-y-3 p-5 text-sm">
                {result && (
                    <div className={calm ? 'flex items-start gap-2 text-fg' : 'rounded-lg border border-danger/30 bg-danger/5 p-3 text-fg'} data-php-tool-result={result.kind}>
                        {result.kind === 'reconciling' && <span className="mt-0.5 shrink-0"><Spinner size="xs" label={resultText(result)} /></span>}
                        <p className="min-w-0" aria-hidden={result.kind === 'reconciling' ? true : undefined}><KeepWords text={resultText(result)} /></p>
                    </div>
                )}
                {result?.kind === 'reconcileGaveUp' && (
                    <Button variant="secondary" onClick={() => { setResult(null); setToken((n) => n + 1); }} className="max-sm:min-h-11" data-php-tool-retry="">
                        {t('common.retry')}
                    </Button>
                )}
                {info === null ? (
                    <Checking label={t('component.phpTool.checking', { tool: name })} />
                ) : info.state === 'unknown' ? (
                    <CouldNotCheck text={t('component.phpTool.unknown', { tool: name })} onRetry={() => setToken((n) => n + 1)} />
                ) : !state?.installed ? (
                    <p className="text-fg-muted" data-php-tool-state="not-installed">{t('component.phpTool.notInstalled', { tool: name })}</p>
                ) : (
                    <>
                        <p className={state.version && (!state.ready || !state.extensions_present) ? 'font-semibold text-fg' : 'text-fg'}
                            data-php-tool-state={!state.version ? 'none' : !state.version_installed ? 'gone' : stopped ? 'stopped' : proofFailed ? 'proof' : !state.extensions_present ? 'extensions' : 'ready'}>
                            <KeepWords text={!state.version
                                ? t('component.phpTool.none', { tool: name })
                                : !state.version_installed
                                    ? t('component.phpTool.gone', { tool: name, version: state.version })
                                    : stopped
                                        ? t('component.phpTool.stopped', { tool: name, version: state.version })
                                        : proofFailed
                                            ? t('component.phpTool.proofFailed', { tool: name, version: state.version, reason: phpToolCauseText(t, state.proof_cause, '') })
                                            : state.extensions_present
                                                ? t('component.phpTool.current', { tool: name, version: state.version })
                                                : t('component.phpTool.missingExtensions', { tool: name, version: state.version })} />
                        </p>
                        {proofFailed && (
                            <Button variant="primary" onClick={() => void check()} disabled={busy} loading={checking} className="max-sm:min-h-11" data-php-tool-check="">
                                {checking ? t('component.phpTool.checkingAgain') : t('component.phpTool.checkAgain')}
                            </Button>
                        )}
                        {checking && (
                            <div className="flex items-start gap-2 text-sm text-fg-muted" data-php-tool-waiting="check">
                                <span className="mt-0.5 shrink-0"><Spinner size="xs" label={t('component.phpTool.checkWait', { tool: name, version: current })} /></span>
                                <span aria-hidden="true">{t('component.phpTool.checkWait', { tool: name, version: current })}</span>
                            </div>
                        )}
                        {options.length === 0 ? (
                            <p className="text-fg-muted" data-php-tool-options="none">{t('component.phpTool.onlyOne', { tool: name })}</p>
                        ) : (
                            <div className="flex flex-wrap items-center gap-2">
                                <label className="flex items-center gap-2 text-fg">
                                    <span>{t('component.phpTool.choose')}</span>
                                    <select
                                        value={selected}
                                        onChange={(event) => setChoice(event.target.value)}
                                        disabled={busy}
                                        className="rounded-lg border border-border-strong bg-surface px-2 py-1 font-mono text-sm text-fg max-sm:min-h-11"
                                        data-php-tool-select=""
                                    >
                                        {options.map((version) => <option key={version} value={version}>PHP {version}</option>)}
                                    </select>
                                </label>
                                <Button variant="secondary" onClick={() => void move()} disabled={busy || !selected} loading={moving} className="max-sm:min-h-11">
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
