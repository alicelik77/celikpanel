// The account's own action log (alpha.128, G23): what was done in this
// account, newest first, as plain sentences. The server decides which rows a
// person may read (GET /api/v1/me/activity, cmd/panel/activity_log.go): the
// person's own, the rows done as them through "Log in as" (said as the
// administrator or the provider), a customer's team members', a reseller's
// customers'. No details are sent; the address is shown only on one's own
// rows. Read only; "Show older" reads the next page. The administrator's
// whole-server history stays the Audit log.
//
// Hesabın kendi işlem günlüğü: hangi satırları göreceğine sunucu karar verir;
// ayrıntı gönderilmez, adres yalnız kişinin kendi satırında görünür. Yalnız okur.
import { useState } from 'react';
import { History } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { Link } from '../router';
import { auditActionParts, auditActionText, auditResourceText } from '../lib/auditAction';
import { readRemote, useRemote } from '../lib/remote';
import { useDomainOpsWords } from '../i18n/domainOps';
import type { DomainOpsKey } from '../i18n/domainOps/en';
import { Button, KnownEmpty, RemoteGate } from './ui';
import { PageHeader } from './PageHeader';

export type ActivityActorKind = 'you' | 'administrator' | 'provider' | 'account' | 'team_member' | 'system' | 'someone';
const ACTOR_KINDS: readonly ActivityActorKind[] = ['you', 'administrator', 'provider', 'account', 'team_member', 'system', 'someone'];

export interface ActivityEntry {
    id: number;
    at: string;
    action: string;
    resource_type?: string;
    resource_id?: number;
    actor: { kind: ActivityActorKind; name?: string };
    as?: string;
    ip?: string;
}

export interface ActivityPageAnswer {
    entries: ActivityEntry[];
    nextBefore: number | null;
    incomplete: boolean;
}

export const ACTIVITY_URL = '/api/v1/me/activity';

// A shape this page does not know is not known (D-024), never an empty list.
export function decodeActivity(raw: unknown): ActivityPageAnswer {
    const body = raw as { entries?: unknown; next_before?: unknown; incomplete?: unknown } | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.entries)) throw new Error('shape');
    const entries = body.entries.map((item) => {
        const entry = item as ActivityEntry;
        if (!entry || typeof entry.id !== 'number' || typeof entry.action !== 'string' || !entry.actor || !ACTOR_KINDS.includes(entry.actor.kind)) {
            throw new Error('entry');
        }
        return entry;
    });
    const nextBefore = typeof body.next_before === 'number' ? body.next_before : null;
    return { entries, nextBefore, incomplete: body.incomplete === true };
}

type Say = (key: DomainOpsKey, values?: Record<string, string | number>) => string;

export function activityActorText(entry: ActivityEntry, say: Say): string {
    const { kind, name } = entry.actor;
    const who = kind === 'account' ? (name || say('activity.actor.someone'))
        : kind === 'team_member' ? say('activity.actor.team_member', { name: name || '—' })
            : say(`activity.actor.${kind}` as DomainOpsKey);
    return entry.as ? say('activity.as', { who, name: entry.as }) : who;
}

function formatTime(at: string, locale: string): string {
    const parsed = new Date(at.includes('T') ? at : `${at.replace(' ', 'T')}Z`);
    return Number.isNaN(parsed.getTime()) ? at : parsed.toLocaleString(locale);
}

export function ActivityPage() {
    const { say, t, locale } = useDomainOpsWords();
    const { role } = useAuth();
    const first = useRemote(ACTIVITY_URL, decodeActivity);
    // Older pages, read on request; a failed read keeps what is shown.
    const [older, setOlder] = useState<{ after: number; entries: ActivityEntry[]; next: number | null } | null>(null);
    const [olderBusy, setOlderBusy] = useState(false);
    const [olderFailed, setOlderFailed] = useState(false);
    const subtitle = role === 'reseller' ? 'activity.subtitleReseller'
            : role === 'additional_user' ? 'activity.subtitleTeam'
                : 'activity.subtitle';

    return (
        <div className="p-4 sm:p-6 md:p-8">
            <PageHeader title={say('activity.title')} subtitle={say(subtitle)} breadcrumb={[t('common.home'), say('activity.title')]} />
            <RemoteGate remote={first.remote} checking={say('activity.checking')} failed={say('activity.unknown')} onRetry={() => { setOlder(null); void first.retry(); }} busy={first.reading}>
                {(shown) => {
                    const page = shown.value;
                    // Older pages belong to the first page they continue.
                    const continued = older && older.after === page.nextBefore ? older : null;
                    const entries = continued ? [...page.entries, ...continued.entries] : page.entries;
                    const next = continued ? continued.next : page.nextBefore;
                    const showOlder = async () => {
                        if (next === null || olderBusy) return;
                        setOlderBusy(true);
                        setOlderFailed(false);
                        const read = await readRemote(`${ACTIVITY_URL}?before=${next}`, decodeActivity);
                        setOlderBusy(false);
                        if (read.state !== 'known') {
                            setOlderFailed(true);
                            return;
                        }
                        setOlder({
                            after: page.nextBefore as number,
                            entries: [...(continued?.entries ?? []), ...read.value.entries],
                            next: read.value.nextBefore,
                        });
                    };
                    if (entries.length === 0) return <KnownEmpty of={shown} icon={History} title={say('activity.empty')} />;
                    return (
                        <>
                            {page.incomplete && (
                                <p role="status" className="mb-4 rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2.5 text-sm text-fg">{say('activity.incomplete')}</p>
                            )}
                            <div className="overflow-x-auto rounded-xl border border-border-strong bg-surface">
                                <table className="w-full text-sm">
                                    <thead>
                                        <tr className="border-b border-border text-left text-xs font-semibold text-fg-muted">
                                            <th className="px-4 py-2.5">{say('activity.col.time')}</th>
                                            <th className="px-4 py-2.5">{say('activity.col.who')}</th>
                                            <th className="px-4 py-2.5">{say('activity.col.what')}</th>
                                            <th className="px-4 py-2.5">{say('activity.col.ip')}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {entries.map((entry) => {
                                            const sentence = entry.action ? auditActionText(entry.action, t) : say('activity.generic');
                                            const about = entry.action && auditActionParts(entry.action).target === ''
                                                ? auditResourceText(entry.resource_type, entry.resource_id, t) : '';
                                            return (
                                                <tr key={entry.id} className="border-b border-border last:border-0" data-activity-row={entry.id}>
                                                    <td className="whitespace-nowrap px-4 py-3 text-fg-muted">{formatTime(entry.at, locale)}</td>
                                                    <td className="px-4 py-3 font-medium text-fg" data-activity-actor={entry.actor.kind}>{activityActorText(entry, say)}</td>
                                                    <td className="px-4 py-3 text-fg">
                                                        {sentence}
                                                        {about && <span className="block text-xs text-fg-subtle">{about}</span>}
                                                    </td>
                                                    <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-fg-subtle">{entry.ip || '—'}</td>
                                                </tr>
                                            );
                                        })}
                                    </tbody>
                                </table>
                            </div>
                            <div className="mt-3 flex flex-wrap items-center gap-3 text-sm">
                                {next !== null ? (
                                    <Button onClick={() => void showOlder()} loading={olderBusy} disabled={olderBusy}>{say('activity.older')}</Button>
                                ) : (
                                    <span className="text-fg-subtle">{say('activity.end')}</span>
                                )}
                                {olderFailed && <span role="status" className="text-fg-muted">{say('activity.olderFailed')}</span>}
                            </div>
                        </>
                    );
                }}
            </RemoteGate>
        </div>
    );
}

// The card on My account that leads here (reseller and customer).
// Hesabım'daki, buraya götüren kart.
export function ActivityCard({ team = false }: { team?: boolean }) {
    const { say } = useDomainOpsWords();
    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-account-section="activity">
            <h2 className="flex items-center gap-2 text-base font-semibold text-fg">
                <History className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                {say('activity.cardTitle')}
            </h2>
            <p className="mt-0.5 text-sm text-fg-muted">{say(team ? 'activity.cardTextTeam' : 'activity.cardText')}</p>
            <p className="mt-3 text-sm">
                <Link to="/audit" className="font-medium text-primary underline underline-offset-2 hover:text-primary-hover">{say('activity.cardOpen')}</Link>
            </p>
        </section>
    );
}

// The Dashboard's "Recent activity" for a reseller or customer (alpha.128,
// G23 follow-up): the newest five rows of the same scoped log, and the way
// to all of it. A read that failed says so; it is never "nothing was done".
// Panodaki "Son etkinlik": aynı kapsamlı günlüğün en yeni beş satırı.
export const RECENT_ACTIVITY_URL = `${ACTIVITY_URL}?limit=5`;

export function RecentActivityCard() {
    const { say, t, locale } = useDomainOpsWords();
    const read = useRemote(RECENT_ACTIVITY_URL, decodeActivity);
    return (
        <section className="mt-6 rounded-xl border border-border bg-surface p-5" data-dashboard-section="recent-activity">
            <div className="mb-3 flex flex-wrap items-center gap-2">
                <History className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{say('activity.recentTitle')}</h2>
                <Link to="/audit" className="ml-auto text-sm font-medium text-primary underline underline-offset-2 hover:text-primary-hover max-sm:min-h-11">{say('activity.recentAll')}</Link>
            </div>
            <RemoteGate remote={read.remote} checking={say('activity.checking')} failed={say('activity.unknown')} onRetry={() => void read.retry()} busy={read.reading}>
                {(shown) => shown.value.entries.length === 0
                    ? <KnownEmpty of={shown} icon={History} title={say('activity.empty')} />
                    : (
                        <ul className="divide-y divide-border text-sm">
                            {shown.value.entries.slice(0, 5).map((entry) => (
                                <li key={entry.id} className="flex flex-wrap items-baseline gap-x-2 py-2" data-recent-activity={entry.id}>
                                    <span className="font-medium text-fg">{activityActorText(entry, say)}</span>
                                    <span className="text-fg-muted">{entry.action ? auditActionText(entry.action, t) : say('activity.generic')}</span>
                                    <span className="ml-auto text-xs text-fg-subtle">{formatTime(entry.at, locale)}</span>
                                </li>
                            ))}
                        </ul>
                    )}
            </RemoteGate>
        </section>
    );
}
