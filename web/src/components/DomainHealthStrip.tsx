// The domain health strip (alpha.128, G22): one compact card on a domain's
// Overview. Each check is fine, or says in plain words what is wrong and
// shows the one action that fixes it: the tab or page where the fix is, or,
// when the fix is someone else's, whom to contact. A check whose answer the
// panel does not have says so and is never shown as fine (D-024).
//
// The answers are the panel's own stored results (GET
// /api/v1/domains/{id}/health, cmd/panel/domain_health.go); "points here" is
// the Connection card's own answer on this page, read with the card's request
// (one request for both). Problems come first; what is fine is one short row.
//
// Alan adı sağlık şeridi: her denetim ya yolundadır ya da neyin yanlış
// olduğunu düz sözle söyler ve düzelten tek eylemi gösterir. Bilinmeyen yanıt
// asla "yolunda" gösterilmez. Sorunlar önce gelir.
import { Activity } from 'lucide-react';
import { Link } from '../router';
import { RemoteGate, StatusDot } from './ui';
import { lastKnown, useInvalidation, useRemote } from '../lib/remote';
import { useDomainOpsWords, isDomainOpsKey } from '../i18n/domainOps';
import type { DomainOpsKey } from '../i18n/domainOps/en';
import { decodeConnection } from './DomainConnection';

export type HealthState = 'ok' | 'problem' | 'waiting' | 'unknown';
export type HealthContact = 'administrator' | 'provider' | 'owner';

export interface HealthFix {
    kind: 'tab' | 'page' | 'contact';
    tab?: string;
    sub?: string;
    path?: string;
}

export interface HealthCheck {
    id: string;
    state: HealthState;
    code: string;
    at?: string;
    earlier?: boolean;
    reason?: string;
    days?: number;
    percent?: number;
    used_bytes?: number;
    limit_bytes?: number;
    items?: string[];
    fix?: HealthFix;
}

export interface DomainHealth {
    contact: HealthContact;
    checks: HealthCheck[];
}

const STATES: readonly HealthState[] = ['ok', 'problem', 'waiting', 'unknown'];
const CONTACTS: readonly HealthContact[] = ['administrator', 'provider', 'owner'];

// A shape this screen does not know is not guessed at: the answer is then
// not known (D-024).
export function decodeDomainHealth(raw: unknown): DomainHealth {
    const body = raw as { contact?: unknown; checks?: unknown } | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.checks) || !CONTACTS.includes(body.contact as HealthContact)) {
        throw new Error('shape');
    }
    const checks = body.checks.map((item) => {
        const check = item as HealthCheck;
        if (!check || typeof check.id !== 'string' || typeof check.code !== 'string' || !STATES.includes(check.state)) throw new Error('check');
        // A code without words is never drawn as fine.
        if (!isDomainOpsKey(`dhealth.${check.id}.${check.code}`)) return { ...check, state: 'unknown' as const, fix: undefined };
        return check;
    });
    return { contact: body.contact as HealthContact, checks };
}

function formatBytes(bytes = 0): string {
    if (bytes < 1024) return `${bytes} B`;
    const units = ['KB', 'MB', 'GB', 'TB'];
    let value = bytes / 1024;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit += 1; }
    return `${value.toFixed(value < 10 ? 1 : 0)} ${units[unit]}`;
}

type Say = (key: DomainOpsKey, values?: Record<string, string | number>) => string;

// The sentence of one check. A code this screen has no words for (a newer
// panel) is said as not known, never as fine.
export function healthSentence(check: HealthCheck, contact: HealthContact, say: Say): string {
    const values: Record<string, string | number> = {
        days: check.days ?? 0,
        percent: check.percent ?? 0,
        used: formatBytes(check.used_bytes),
        limit: formatBytes(check.limit_bytes),
        items: (check.items ?? []).map((item) => item.toUpperCase()).join(', '),
        who: say(`dhealth.who.${contact}` as DomainOpsKey),
    };
    if (check.id === 'site' && check.code === 'down') {
        const reason = `dhealth.reason.${check.reason ?? 'error'}`;
        values.reason = say(isDomainOpsKey(reason) ? reason : 'dhealth.reason.error');
    }
    let key = `dhealth.${check.id}.${check.code}`;
    if (check.id === 'backup' && (check.code === 'recent' || check.code === 'recent_manual') && (check.days ?? 0) < 1) key += 'Today';
    if (!isDomainOpsKey(key)) return say('dhealth.unknownCode');
    return say(key, values);
}

const TONE: Record<HealthState, 'success' | 'danger' | 'unknown' | 'idle'> = {
    ok: 'success',
    problem: 'danger',
    waiting: 'idle',
    unknown: 'unknown',
};

interface Props {
    domainId: number;
    /** The Connection card is on this page: its answer is the "points here" line. */
    showsConnection: boolean;
    /** Opens a tab of this page, when the page has it. */
    hasTab: (tab: string, sub?: string) => boolean;
    onGo: (tab: string, sub?: string) => void;
}

export function DomainHealthStrip({ domainId, showsConnection, hasTab, onGo }: Props) {
    const { say } = useDomainOpsWords();
    const health = useRemote(`/api/v1/domains/${domainId}/health`, decodeDomainHealth);
    // A change on this page (a certificate, mail records, a backup schedule)
    // reads the strip again; it only reads.
    useInvalidation(['domains', 'dns', 'mail'], () => void health.retry());
    const connection = useRemote(showsConnection ? `/api/v1/domains/${domainId}/connection` : null, decodeConnection, { init: { cache: 'no-store' } });
    const conn = showsConnection ? lastKnown(connection.remote) : undefined;

    return (
        <section className="mb-4 rounded-xl border border-border bg-surface p-4" data-domain-health="">
            <h2 className="flex items-center gap-2 text-sm font-semibold text-fg">
                <Activity className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                {say('dhealth.title')}
            </h2>
            <div className="mt-2">
                <RemoteGate remote={health.remote} checking={say('dhealth.checking')} failed={say('dhealth.unknown')} onRetry={() => void health.retry()} busy={health.reading}>
                    {({ value }) => {
                        const checks: HealthCheck[] = [...value.checks];
                        if (showsConnection) {
                            const status = conn?.value.status;
                            const here = status === 'delegated' || status === 'a_record';
                            const known = status !== undefined && status !== 'unknown';
                            checks.splice(Math.min(2, checks.length), 0, {
                                id: 'dns',
                                state: here ? 'ok' : known ? 'problem' : 'unknown',
                                code: here ? 'here' : known ? 'notHere' : 'unknown',
                            });
                        }
                        const order: HealthState[] = ['problem', 'unknown', 'waiting', 'ok'];
                        const attention = checks.filter((check) => check.state !== 'ok')
                            .sort((a, b) => order.indexOf(a.state) - order.indexOf(b.state));
                        const fine = checks.filter((check) => check.state === 'ok');
                        return (
                            <>
                                {attention.length > 0 && (
                                    <ul className="space-y-1.5 text-sm" data-health-attention="">
                                        {attention.map((check) => (
                                            <li key={check.id} className="flex flex-wrap items-baseline gap-x-2 gap-y-1" data-health-check={check.id} data-health-state={check.state}>
                                                <span className="self-center"><StatusDot tone={TONE[check.state]} /></span>
                                                <span className="text-fg">
                                                    {healthSentence(check, value.contact, say)}
                                                    {check.earlier && <span className="ml-1 text-fg-subtle">{say('dhealth.earlier')}</span>}
                                                </span>
                                                <FixAction check={check} contact={value.contact} say={say} hasTab={hasTab} onGo={onGo} />
                                            </li>
                                        ))}
                                    </ul>
                                )}
                                {fine.length > 0 && (
                                    <ul className={`flex flex-wrap gap-x-4 gap-y-1 text-sm text-fg-muted ${attention.length > 0 ? 'mt-2 border-t border-border pt-2' : ''}`} data-health-fine="">
                                        {fine.map((check) => (
                                            <li key={check.id} className="inline-flex items-center gap-1.5" data-health-check={check.id} data-health-state="ok">
                                                <StatusDot tone="success" />
                                                {healthSentence(check, value.contact, say)}
                                            </li>
                                        ))}
                                    </ul>
                                )}
                                {attention.length === 0 && fine.length > 0 && <p className="mt-1 text-xs text-fg-subtle">{say('dhealth.allGood')}</p>}
                                <p className="mt-2 text-xs text-fg-subtle">{say('dhealth.note')}</p>
                            </>
                        );
                    }}
                </RemoteGate>
            </div>
        </section>
    );
}

function FixAction({ check, contact, say, hasTab, onGo }: {
    check: HealthCheck;
    contact: HealthContact;
    say: Say;
    hasTab: (tab: string, sub?: string) => boolean;
    onGo: (tab: string, sub?: string) => void;
}) {
    const fix = check.fix;
    if (!fix || check.state === 'ok') return null;
    const linkClass = 'rounded font-medium text-primary underline underline-offset-2 hover:text-primary-hover';
    const contactLine = <span className="text-fg-muted" data-health-fix="contact">{say(`dhealth.contact.${contact}` as DomainOpsKey)}</span>;
    if (fix.kind === 'tab' && fix.tab) {
        // A tab this page does not draw (a right closed since) is not a
        // way to the fix: whoever can open it is named instead.
        if (!hasTab(fix.tab, fix.sub)) return contactLine;
        const key = `dhealth.fix.${fix.tab}${fix.sub ? `.${fix.sub}` : ''}`;
        const label = isDomainOpsKey(key) ? say(key) : fix.tab;
        // Making room is the owner's own action, more room is the provider's.
        const room = check.id === 'disk' ? <span className="text-fg-muted">{say('dhealth.disk.makeRoom', { who: say(`dhealth.who.${contact}` as DomainOpsKey) })}</span> : null;
        return (
            <>
                {room}
                <button type="button" className={linkClass} onClick={() => onGo(fix.tab!, fix.sub)} data-health-fix="tab">{label}</button>
            </>
        );
    }
    if (fix.kind === 'page' && fix.path && fix.path.startsWith('/') && !fix.path.startsWith('//')) {
        const key = `dhealth.fix.page.${fix.path}`;
        return <Link to={fix.path} className={linkClass} data-health-fix="page">{isDomainOpsKey(key) ? say(key) : fix.path}</Link>;
    }
    return contactLine;
}
