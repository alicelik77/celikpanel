import type { ReactNode } from 'react';
import { Clock, Cpu, Power } from 'lucide-react';
import { useMaintenanceWords } from '../i18n/serverMaintenance';
import { readApiError, refusalText } from '../lib/apiError';
import { confirmAction, type OperationEnd, type OperationReport, type OperationStep } from '../lib/confirmAction';
import { sendWithSteps, type StepLabel } from '../lib/requestProgress';
import { readRemote, useRemote } from '../lib/remote';
import { formatBytes } from '../lib/usage';
import { Button, KeepWords, RemoteGate } from './ui';
import { LifeLine, lifeDate } from './LifeLine';

// Server → Maintenance for the administrator (alpha.128, G29): the server's
// clock, its busiest processes and restarting it. The reads change nothing
// (cmd/panel/server_maintenance.go). The restart asks with the panel's own
// dialogue, then the progress window shows the server's steps (checks, the
// action log, the request) and waits, saying so, until the panel answers
// again and its start time shows the server did restart. A lost answer, a
// server that never stopped and one that never came back are each said as
// what they are (D-024). There is no shutdown and no "kill" here.
//
// Sunucu → Bakım: saat, en yoğun işlemler ve yeniden başlatma. Okumalar
// hiçbir şey değiştirmez; yeniden başlatma adım adım izlenir.

const CLOCK_URL = '/api/v1/maintenance/clock';
const PROCESSES_URL = '/api/v1/maintenance/processes';
const REBOOT_URL = '/api/v1/maintenance/reboot';
// Public and in memory on the server: it answers as soon as the panel runs.
const ALIVE_URL = '/api/v1/panel/access-address';

export interface ServerClock {
    now: string;
    time_zone: string;
    utc_offset: string;
    sync: 'synchronised' | 'not_synchronised' | 'unknown';
    boot_id?: string;
    booted_at?: string;
}

export function decodeServerClock(raw: unknown): ServerClock {
    const body = raw as Partial<ServerClock> | null;
    if (!body || typeof body.now !== 'string' || typeof body.utc_offset !== 'string' || typeof body.sync !== 'string') throw new Error('shape');
    return { ...body, time_zone: typeof body.time_zone === 'string' ? body.time_zone : '' } as ServerClock;
}

interface ProcessLine { pid: number; name: string; user?: string; cpu: number; rss: number }
export interface ServerProcesses {
    state: 'read' | 'hidden';
    total?: number;
    vcpus?: number;
    by_cpu: ProcessLine[];
    by_memory: ProcessLine[];
}

export function decodeServerProcesses(raw: unknown): ServerProcesses {
    const body = raw as Partial<ServerProcesses> | null;
    if (!body || (body.state !== 'read' && body.state !== 'hidden') || !Array.isArray(body.by_cpu) || !Array.isArray(body.by_memory)) throw new Error('shape');
    return body as ServerProcesses;
}

function Card({ icon: Icon, title, desc, action, attr, children }: {
    icon: typeof Clock; title: string; desc: string; action?: ReactNode; attr: string; children: ReactNode;
}) {
    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-maintenance-section={attr}>
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 flex-1 basis-64">
                    <h2 className="flex items-center gap-2 text-base font-semibold text-fg">
                        <Icon className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                        {title}
                    </h2>
                    <p className="mt-0.5 text-sm text-fg-muted">{desc}</p>
                </div>
                {action}
            </div>
            <div className="mt-4">{children}</div>
        </section>
    );
}

export function ServerClockCard() {
    const { t, locale } = useMaintenanceWords();
    const handle = useRemote(CLOCK_URL, decodeServerClock);
    return (
        <Card icon={Clock} title={t('mclock.title')} desc={t('mclock.desc')} attr="clock"
            action={handle.remote.state !== 'loading' && (
                <Button onClick={() => void handle.retry()} loading={handle.reading} disabled={handle.reading} className="max-sm:min-h-11">{t('mclock.again')}</Button>
            )}>
            <RemoteGate remote={handle.remote} checking={t('mclock.checking')} failed={t('mclock.failed')} onRetry={() => void handle.retry()} busy={handle.reading}>
                {({ value }) => (
                    <div className="space-y-2" data-clock-sync={value.sync}>
                        <LifeLine tone="info" attr="clock-now">{t('mclock.now', { time: lifeDate(value.now, locale, true) })}</LifeLine>
                        <LifeLine tone="info" attr="clock-zone">
                            {value.time_zone ? t('mclock.zone', { zone: value.time_zone, offset: value.utc_offset }) : t('mclock.zoneUnknown', { offset: value.utc_offset })}
                        </LifeLine>
                        {value.booted_at && <LifeLine tone="info" attr="clock-booted">{t('mclock.booted', { time: lifeDate(value.booted_at, locale, true) })}</LifeLine>}
                        {value.sync === 'synchronised'
                            ? <LifeLine tone="ok" attr="clock-synced">{t('mclock.synced')}</LifeLine>
                            : value.sync === 'not_synchronised'
                                ? <LifeLine tone="warn" attr="clock-not-synced"><KeepWords text={t('mclock.notSynced')} /></LifeLine>
                                : <LifeLine tone="info" attr="clock-sync-unknown"><KeepWords text={t('mclock.syncUnknown')} /></LifeLine>}
                        <p className="pl-6 text-sm text-fg-subtle"><KeepWords text={t('mclock.zoneHint')} /></p>
                    </div>
                )}
            </RemoteGate>
        </Card>
    );
}

function ProcessList({ title, lines, kind }: { title: string; lines: ProcessLine[]; kind: 'cpu' | 'memory' }) {
    const { t } = useMaintenanceWords();
    return (
        <div className="min-w-0" data-process-list={kind}>
            <h3 className="text-sm font-semibold text-fg">{title}</h3>
            {lines.length === 0 ? <p className="mt-2 text-sm text-fg-muted">{t('mproc.empty')}</p> : (
                <table className="mt-2 w-full table-fixed text-sm">
                    <thead>
                        <tr className="text-left text-xs text-fg-subtle">
                            <th className="w-[45%] pb-1 font-medium">{t('mproc.colName')}</th>
                            <th className="w-[30%] pb-1 font-medium">{t('mproc.colUser')}</th>
                            <th className="pb-1 text-right font-medium">{t('mproc.colValue')}</th>
                        </tr>
                    </thead>
                    <tbody>
                        {lines.map((line) => (
                            <tr key={line.pid} className="border-t border-border/60">
                                <td className="truncate py-1 font-mono text-fg" title={`PID ${line.pid}`}>{line.name}</td>
                                <td className="truncate py-1 text-fg-muted">{line.user ?? ''}</td>
                                <td className="py-1 text-right tabular-nums text-fg">{kind === 'cpu' ? t('mproc.cpu', { value: line.cpu.toFixed(1) }) : formatBytes(line.rss)}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            )}
        </div>
    );
}

export function ServerProcessesCard() {
    const { t } = useMaintenanceWords();
    const handle = useRemote(PROCESSES_URL, decodeServerProcesses);
    return (
        <Card icon={Cpu} title={t('mproc.title')} desc={t('mproc.desc', { n: 15 })} attr="processes"
            action={handle.remote.state !== 'loading' && (
                <Button onClick={() => void handle.retry()} loading={handle.reading} disabled={handle.reading} className="max-sm:min-h-11">{t('mproc.again')}</Button>
            )}>
            <RemoteGate remote={handle.remote} checking={t('mproc.checking')} failed={t('mproc.failed')} onRetry={() => void handle.retry()} busy={handle.reading}>
                {({ value }) => value.state === 'hidden'
                    ? <LifeLine tone="info" attr="processes-hidden"><KeepWords text={t('mproc.hidden')} /></LifeLine>
                    : (
                        <div className="space-y-3">
                            <p className="text-sm text-fg-muted">{t('mproc.total', { n: value.total ?? 0, cpus: value.vcpus ?? 1 })}</p>
                            <div className="grid gap-5 md:grid-cols-2">
                                <ProcessList title={t('mproc.byCPU')} lines={value.by_cpu} kind="cpu" />
                                <ProcessList title={t('mproc.byMemory')} lines={value.by_memory} kind="memory" />
                            </div>
                            <p className="text-sm text-fg-subtle"><KeepWords text={t('mproc.noKill')} /></p>
                        </div>
                    )}
            </RemoteGate>
        </Card>
    );
}

// --- Restart ---------------------------------------------------------------

const pause = (ms: number) => new Promise<void>((resolve) => { setTimeout(resolve, ms); });
const POLL_MS = 2000;
// A server that still answers this long after the request did not restart.
const DOWN_LIMIT_MS = 3 * 60_000;
// A server that does not come back within this is said as not known.
const UP_LIMIT_MIN = 10;
const TYPICAL_RESTART_S = 90;

/** Whether the panel answers now (a public read the panel serves from memory). */
async function panelAnswers(): Promise<boolean> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 4000);
    const read = await readRemote(ALIVE_URL, () => true, undefined, { cache: 'no-store', credentials: 'same-origin', signal: controller.signal });
    clearTimeout(timer);
    return read.state === 'known';
}

/** The clock after the restart: its boot identity and start time, or unknown. */
async function readClock(): Promise<ServerClock | null> {
    const read = await readRemote(CLOCK_URL, decodeServerClock, undefined, { cache: 'no-store', credentials: 'same-origin' });
    return read.state === 'known' ? read.value : null;
}

export function ServerRebootCard() {
    const { t, locale } = useMaintenanceWords();
    const serverSteps: StepLabel[] = [
        { id: 'check', label: t('mreboot.step.check'), check: true },
        { id: 'record', label: t('mreboot.step.record') },
        { id: 'schedule', label: t('mreboot.step.schedule') },
    ];
    const waitSteps = (at: 'down' | 'up' | 'verify', since: number, note?: string): OperationStep[] => {
        const order = ['down', 'up', 'verify'] as const;
        const index = order.indexOf(at);
        return [
            ...serverSteps.map((step) => ({ id: step.id, label: step.label, state: 'done' as const })),
            ...order.map((id, position) => ({
                id, label: t(`mreboot.step.${id}`),
                state: position < index ? 'done' as const : position === index ? 'running' as const : 'pending' as const,
                ...(position === index ? { since, note } : {}),
            })),
        ];
    };

    const restart = async (report: OperationReport): Promise<OperationEnd> => {
        const res = await sendWithSteps(report, serverSteps, t, 'maintenance.reboot', async (headers): Promise<Response | null> => {
            try {
                return await fetch(REBOOT_URL, { method: 'POST', headers: { 'Content-Type': 'application/json', ...headers }, body: '{}' });
            } catch {
                // No answer: whether the server is restarting is not known (said below).
                return null;
            }
        });
        if (!res) return { state: 'unknown', reason: t('mreboot.lost') };
        if (!res.ok) {
            const problem = await readApiError(res);
            const reason = refusalText(problem, t, 'mreboot.refused' as never);
            return { state: res.status === 409 ? 'blocked' : res.status === 502 ? 'unknown' : 'failed', reason };
        }
        // Without the boot identity in the answer the start time still tells.
        const answer = await res.json().then((body: { boot_id?: unknown }) => body, () => ({ boot_id: '' }));
        const before = typeof answer.boot_id === 'string' ? answer.boot_id : '';
        // 1. Wait for the panel to stop answering.
        let since = Date.now();
        report.steps(waitSteps('down', since, t('mreboot.downNote')));
        let wentDown = false;
        while (Date.now() - since < DOWN_LIMIT_MS) {
            await pause(POLL_MS);
            if (!(await panelAnswers())) {
                wentDown = true;
                break;
            }
        }
        if (!wentDown) {
            const clock = await readClock();
            if (clock && before && clock.boot_id === before) return { state: 'failed', reason: t('mreboot.notRestarted') };
            // It answered all along, yet the start changed (a very fast start) or cannot be read.
            if (clock?.boot_id && before && clock.boot_id !== before) {
                return { state: 'done', message: t('mreboot.done', { time: lifeDate(clock.booted_at, locale, true) }) };
            }
            return { state: 'unknown', reason: t('mreboot.notRestarted') };
        }
        // 2. Wait for it to answer again, saying how long it usually takes.
        since = Date.now();
        report.steps(waitSteps('up', since, t('mreboot.upNote', { seconds: TYPICAL_RESTART_S })));
        let back = false;
        while (Date.now() - since < UP_LIMIT_MIN * 60_000) {
            await pause(POLL_MS);
            if (await panelAnswers()) {
                back = true;
                break;
            }
            const left = TYPICAL_RESTART_S - Math.floor((Date.now() - since) / 1000);
            report.steps(waitSteps('up', since, left > 0 ? t('mreboot.upNote', { seconds: left }) : t('mreboot.upLate')));
        }
        if (!back) return { state: 'unknown', reason: t('mreboot.noReturn', { minutes: UP_LIMIT_MIN }) };
        // 3. The start time says it restarted.
        report.steps(waitSteps('verify', Date.now()));
        const clock = await readClock();
        if (clock?.boot_id && before && clock.boot_id === before) return { state: 'failed', reason: t('mreboot.notRestarted') };
        if (clock?.booted_at) return { state: 'done', message: t('mreboot.done', { time: lifeDate(clock.booted_at, locale, true) }) };
        return { state: 'done', message: t('mreboot.doneUnverified') };
    };

    return (
        <Card icon={Power} title={t('mreboot.title')} desc={t('mreboot.desc')} attr="reboot">
            <Button variant="danger" className="max-sm:min-h-11" data-reboot-offer=""
                onClick={() => void confirmAction({
                    title: t('mreboot.confirmTitle'),
                    message: t('mreboot.confirmMessage'),
                    confirmLabel: t('mreboot.confirm'),
                    tone: 'danger',
                    progressTitle: t('mreboot.progress'),
                    action: restart,
                })}>
                {t('mreboot.button')}
            </Button>
        </Card>
    );
}
