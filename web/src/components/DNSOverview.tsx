import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Link } from '../router';
import { apiErrorText, readApiError } from '../lib/apiError';
import { useLostAnswer } from '../lib/lostAnswer';
import { readRemote, useRemote, type Remote, type RemoteHandle } from '../lib/remote';
import {
    DNS_OVERVIEW_URL,
    decodeDNSOverview,
    decodeDNSRecordRows,
    dnsOverviewMXTarget,
    dnsOverviewSentenceKey,
    dnsOverviewSentenceValues,
    dnsOverviewServerLabel,
    dnsOverviewTone,
    type DNSOverview as Overview,
    type DNSOverviewMail,
    type DNSOverviewName,
    type DNSOverviewServer,
    type DNSRecordRow,
} from '../lib/dnsOverview';
import { Button } from './ui';
import { SetupJoinCard } from './ServerSetupWizardSteps';

// The DNS page (v0.1.0-alpha.115, the owner-approved draft): one sentence
// first (why, who acts, the next action, what happens then), the DNS servers,
// the names with what the internet answers for each, the main domain's mail
// records, where your own records are edited, and today's technical screen
// collapsed under Advanced. It only reads GET /api/v1/dns/overview, about once
// a minute; the server shares one bounded answer, so polling never asks the
// internet faster and never changes anything. The one change it offers,
// "Move mail to …", is the existing record editor's MX change after an
// in-page confirmation that states the consequence; a lost answer is never
// sent again (lib/lostAnswer.ts). "Move main DNS" is not part of this release.
// DNS sayfasi: once tek cumle, sonra sunucular, adlar, posta ve Gelismis.

type T = (key: string, values?: Record<string, string | number>) => string;

const toneClass = {
    bad: 'border-danger/40 bg-danger/5',
    task: 'border-warning-mark/50 bg-warning-mark/10',
    done: 'border-success/40 bg-success/5',
    neutral: 'border-border bg-surface',
} as const;

const stateClass = (state: string) => state === 'ok' ? 'text-success' : state === 'different' || state === 'not_serving' ? 'text-danger' : 'text-fg-muted';

const softwareName = (id?: string) => id === 'bind' ? 'BIND' : id === 'pdns' ? 'PowerDNS' : id || '';

function rowStateText(t: T, row: Pick<DNSOverviewName, 'state' | 'seen'> & { own?: string; own_seen?: string[] }): string {
    const seen = row.seen.join(', ') || '—';
    if (row.own === 'differs') return t('dnsov.state.ownDiffers', { seen: row.own_seen?.join(', ') || '—' });
    if (row.state === 'not_yet' && row.seen.length > 0) return t('dnsov.state.not_yetSeen', { seen });
    return t(`dnsov.state.${row.state}`, { seen });
}

// The next read follows the server's own next_check_after (its shared answer
// expires then) and waits while the tab is hidden; a read that failed is
// tried again after 30 s. It only reads.
// Sonraki okuma sunucunun next_check_after degerini izler; sekme gizliyken bekler.
function useOverviewPolling(remote: RemoteHandle<Overview>, value: Overview | null) {
    const latest = useRef(remote);
    latest.current = remote;
    const seconds = value ? Math.max(5, value.next_check_after || 60) : 30;
    const stamp = value ? `${value.checked_at}|${remote.remote.state}` : remote.remote.state;
    useEffect(() => {
        const page = typeof document === 'undefined' ? null : document;
        let timer: ReturnType<typeof setTimeout> | undefined;
        const schedule = () => {
            clearTimeout(timer);
            if (page?.visibilityState === 'hidden') return;
            timer = setTimeout(() => { if (!latest.current.reading) void latest.current.retry(); }, seconds * 1000);
        };
        const onVisibility = () => { if (page?.visibilityState === 'visible') schedule(); else clearTimeout(timer); };
        schedule();
        page?.addEventListener('visibilitychange', onVisibility);
        return () => { clearTimeout(timer); page?.removeEventListener('visibilitychange', onVisibility); };
    }, [stamp, seconds]);
}

export function DNSOverview({ advanced }: { advanced: ReactNode }) {
    const i18n = useI18n();
    const t: T = (key, values) => i18n.t(key as TranslationKey, values);
    const remote = useRemote(DNS_OVERVIEW_URL, decodeDNSOverview);
    const [advancedOpen, setAdvancedOpen] = useState(false);
    const [forcing, setForcing] = useState(false);
    const value = remote.remote.state === 'known' ? remote.remote.value : remote.remote.state === 'unknown' ? remote.remote.previous?.value ?? null : null;
    const observedAt = remote.remote.state === 'known' ? remote.remote.observedAt : remote.remote.state === 'unknown' ? remote.remote.previous?.observedAt ?? 0 : 0;
    useOverviewPolling(remote, value);
    // "Check now" asks the server for a real new check; it accepts one per
    // 15 s and otherwise answers with the latest check and when the next one
    // is possible. Until then the button says so.
    const freshAt = value ? observedAt + value.fresh_after * 1000 : 0;
    const [now, setNow] = useState(() => Date.now());
    useEffect(() => {
        if (!freshAt || freshAt <= Date.now()) return;
        const timer = setTimeout(() => setNow(Date.now()), freshAt - Date.now() + 50);
        return () => clearTimeout(timer);
    }, [freshAt]);
    const at = Math.max(now, Date.now());
    const freshIn = freshAt > at ? Math.ceil((freshAt - at) / 1000) : 0;
    const forced = useRef(false);
    async function checkNow() {
        if (forced.current) return;
        forced.current = true;
        setForcing(true);
        try {
            // The forced read refreshes the server's shared answer; the page's
            // own read then shows it (both only read).
            await readRemote(`${DNS_OVERVIEW_URL}?fresh=1`, decodeDNSOverview);
            await remote.retry();
            setNow(Date.now());
        } finally {
            forced.current = false;
            setForcing(false);
        }
    }
    const failed = remote.remote.state === 'unknown';
    const time = (at: string) => new Date(at).toLocaleTimeString(i18n.locale);
    return <div className="space-y-6" data-dns-overview="">
        {!value ? <p role="status" className="text-sm text-fg-muted" data-dns-overview-state={failed ? 'unknown' : 'loading'}>{t(failed ? 'dnsov.readFailed' : 'dnsov.loading')}</p>
            : <OverviewBody value={value} t={t} checkNow={() => void checkNow()} reading={remote.reading || forcing} freshIn={freshIn} failedNote={failed ? t('dnsov.readFailedKept', { time: time(value.checked_at) }) : ''} checkedAt={t('dnsov.checkedAt', { time: time(value.checked_at) })} rereadOverview={remote.retry} />}
        <details className="rounded-lg border border-border bg-surface" onToggle={event => setAdvancedOpen((event.currentTarget as HTMLDetailsElement).open)} data-dns-advanced="">
            <summary className="cursor-pointer px-4 py-3 font-semibold sm:px-5">{t('dnsov.advanced')}</summary>
            <div className="space-y-4 border-t border-border p-4 sm:p-5">
                <p className="text-sm text-fg-muted">{t('dnsov.advancedHelp')}</p>
                {/* Mounted only while open: the technical screen reads the engine itself. */}
                {advancedOpen && advanced}
            </div>
        </details>
    </div>;
}

function OverviewBody({ value, t, checkNow, reading, freshIn, failedNote, checkedAt, rereadOverview }: {
    value: Overview; t: T; checkNow: () => void; reading: boolean; freshIn: number; failedNote: string; checkedAt: string; rereadOverview: () => Promise<Remote<Overview>>;
}) {
    const stem = dnsOverviewSentenceKey(value.sentence.code);
    const vars = dnsOverviewSentenceValues(t, value);
    const say = (part: 'title' | 'why' | 'who' | 'next' | 'then', fallback: string) => stem ? t(`${stem}.${part}`, vars) : fallback;
    const tone = dnsOverviewTone(value.sentence.state);
    // Only the title is a live region. It is an alert only when the state
    // turns into a verified failure while the page is open; a poll that
    // reads the same failure again announces nothing new.
    const previous = useRef(value.sentence.state);
    const [alerting, setAlerting] = useState(false);
    useEffect(() => {
        const state = value.sentence.state;
        if (state === 'failed' && previous.current !== 'failed') setAlerting(true);
        if (state !== 'failed') setAlerting(false);
        previous.current = state;
    }, [value.sentence.state]);
    return <>
        <section aria-labelledby="dns-overview-lead" className={`rounded-lg border p-4 sm:p-5 ${toneClass[tone]}`} data-dns-sentence={value.sentence.code} data-state={value.sentence.state}>
            <div role={alerting ? 'alert' : 'status'} data-dns-sentence-live={alerting ? 'alert' : 'status'}>
                <h2 id="dns-overview-lead" className="text-lg font-semibold">{say('title', value.sentence.reason)}</h2>
            </div>
            <dl className="mt-3 grid gap-x-4 gap-y-2 text-sm leading-6 sm:grid-cols-[minmax(0,9rem)_minmax(0,1fr)]">
                <dt className="text-fg-muted">{t('setup.v2.why')}</dt><dd className="min-w-0 break-words">{say('why', value.sentence.reason)}</dd>
                <dt className="text-fg-muted">{t('setup.v2.who')}</dt><dd className="min-w-0 break-words">{say('who', value.sentence.who)}</dd>
                <dt className="text-fg-muted">{t('setup.v2.next')}</dt><dd className="min-w-0 break-words">{say('next', value.sentence.next)}</dd>
                <dt className="text-fg-muted">{t('setup.v2.then')}</dt><dd className="min-w-0 break-words">{say('then', value.sentence.then)}</dd>
            </dl>
            <div className="mt-3 flex flex-wrap items-center gap-3">
                <Button type="button" variant="secondary" disabled={reading || freshIn > 0} loading={reading} onClick={checkNow}>{t('dnsov.checkNow')}</Button>
                <span className="text-xs text-fg-muted">{freshIn > 0 ? t('dnsov.checkNowIn', { seconds: freshIn }) : checkedAt}</span>
                {['dns_overview_setup_running', 'dns_overview_setup_not_done', 'dns_overview_dkim_pending'].includes(value.sentence.code)
                    ? <Link to="/setup" className="text-sm font-medium text-primary underline underline-offset-4">{t('dnsov.join.openSetup')}</Link> : null}
            </div>
            {failedNote && <p className="mt-2 text-sm text-fg-muted">{failedNote}</p>}
        </section>
        {value.home === 'own' ? <ServersCard value={value} t={t} /> : value.home !== 'none' && <HomeCard value={value} t={t} />}
        {value.join.state !== 'bound' && <JoinEntry value={value} t={t} />}
        <NamesCard value={value} t={t} />
        {value.mail.length > 0 && <MailCard value={value} t={t} rereadOverview={rereadOverview} />}
        <OwnRecordsCard value={value} t={t} />
    </>;
}

function ServerTile({ server, title, t, single }: { server: DNSOverviewServer; title: string; t: T; single?: boolean }) {
    const answers = server.answers === 'ok' && single ? t('dnsov.servers.answers.okSingle') : server.answers === 'not_applicable' ? '' : t(`dnsov.servers.answers.${server.answers}`);
    return <div className="min-w-0 rounded-md border border-border bg-surface-2 p-3" data-dns-server={server.who} data-role={server.role} data-answers={server.answers}>
        <span className="block text-xs text-fg-muted">{title}</span>
        <b className="block break-words">{server.answers === 'not_joined' && !server.short_name ? t('dnsov.peerNotJoined') : dnsOverviewServerLabel(t, server)}</b>
        {server.panel_name && <span className="block break-all font-mono text-xs text-fg-muted">{server.panel_name}{server.ipv4 ? ` · ${server.ipv4}` : ''}</span>}
        {server.role !== 'none' && server.answers !== 'not_joined' && <span className="block text-sm text-fg-muted">{server.software ? t('dnsov.servers.software', { name: softwareName(server.software) }) : t('dnsov.servers.noSoftware')}</span>}
        {answers && <span className={`block text-sm ${stateClass(server.answers)}`}>{answers}</span>}
    </div>;
}

function ServersCard({ value, t }: { value: Overview; t: T }) {
    const single = value.servers.length === 1 && value.servers[0].role === 'standalone';
    const ordered = [...value.servers].sort((a, b) => (a.role === 'main' ? 0 : 1) - (b.role === 'main' ? 0 : 1));
    return <section aria-labelledby="dns-servers-title" className="space-y-3 rounded-lg border border-border bg-surface p-4 sm:p-5">
        <h2 id="dns-servers-title" className="font-semibold">{t('dnsov.servers.title')}</h2>
        <div className="grid gap-3 sm:grid-cols-2">
            {ordered.map(server => <ServerTile key={server.who} server={server} t={t} single={single}
                title={t(single ? 'dnsov.servers.single' : server.role === 'backup' ? 'dnsov.servers.backup' : 'dnsov.servers.main')} />)}
        </div>
        {!single && <p className="text-sm leading-6 text-fg-muted">{t('dnsov.servers.help')}</p>}
    </section>;
}

function HomeCard({ value, t }: { value: Overview; t: T }) {
    const provider = value.home === 'provider';
    return <section aria-labelledby="dns-home-title" className="space-y-2 rounded-lg border border-border bg-surface p-4 sm:p-5" data-dns-home={value.home}>
        <h2 id="dns-home-title" className="font-semibold">{t('dnsov.home.title')}</h2>
        <p className="font-medium">{t(provider ? 'dnsov.home.provider' : 'dnsov.home.remote')}</p>
        <p className="text-sm text-fg-muted">{t(provider ? 'dnsov.home.providerNone' : 'dnsov.home.remoteNone')}</p>
        <p className="text-sm leading-6 text-fg-muted">{t(provider ? 'dnsov.home.providerHelp' : 'dnsov.home.remoteHelp')}</p>
    </section>;
}

function JoinEntry({ value, t }: { value: Overview; t: T }) {
    if (value.join.state === 'waiting') {
        return <section aria-labelledby="dns-join-title" className="space-y-2 rounded-lg border border-border bg-surface p-4 sm:p-5" data-dns-join="waiting">
            <h2 id="dns-join-title" className="font-semibold">{t('dnsov.join.title')}</h2>
            <p className="text-sm leading-6 text-fg-muted">{t('dnsov.join.waiting')}</p>
            {/* The existing join card: it makes the code on request, shown once. */}
            <SetupJoinCard />
        </section>;
    }
    const reason = value.join.reason || 'setup_not_done';
    return <section aria-labelledby="dns-join-title" className="space-y-2 rounded-lg border border-border bg-surface p-4 sm:p-5" data-dns-join={reason}>
        <h2 id="dns-join-title" className="font-semibold">{t('dnsov.join.title')}</h2>
        <p className="text-sm leading-6 text-fg-muted">{t(`dnsov.join.${reason}`)}</p>
        {reason === 'setup_not_done' && <Link to="/setup" className="text-sm font-medium text-primary underline underline-offset-4">{t('dnsov.join.openSetup')}</Link>}
    </section>;
}

function NamesCard({ value, t }: { value: Overview; t: T }) {
    const where = value.names.find(row => row.where !== 'registrar')?.where;
    const mainPanel = value.servers.find(server => server.role === 'main')?.panel_name || '—';
    const serverOf = (row: DNSOverviewName) => row.server ? dnsOverviewServerLabel(t, value.servers.find(server => server.who === row.server) || { who: row.server }) : '';
    return <section aria-labelledby="dns-names-title" className="space-y-3 rounded-lg border border-border bg-surface p-4 sm:p-5">
        <h2 id="dns-names-title" className="font-semibold">{value.domain ? t('dnsov.names.title', { domain: value.domain }) : t('dnsov.names.titleNoDomain')}</h2>
        {value.names.length === 0 ? <p className="text-sm text-fg-muted">{t('dnsov.names.none')}</p>
            : <div role="table" className="divide-y divide-border text-sm">
                <div role="row" className="hidden gap-3 pb-2 text-xs font-medium text-fg-muted sm:grid sm:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)]">
                    <span role="columnheader">{t('dnsov.names.name')}</span><span role="columnheader">{t('dnsov.names.pointsTo')}</span><span role="columnheader">{t('dnsov.names.seen')}</span>
                </div>
                {value.names.map(row => <div role="row" key={`${row.kind}-${row.name}-${row.type}`} className="grid gap-1 py-2 sm:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)] sm:gap-3" data-dns-name={row.name} data-kind={row.kind} data-state={row.state}>
                    <span role="cell" className="min-w-0"><span className="block break-all font-mono">{row.name}</span>
                        <span className="block text-xs text-fg-muted">{t(`dnsov.names.kind.${row.kind}`)}{row.optional ? ` · ${t('dnsov.names.optional')}` : ''}</span></span>
                    <span role="cell" className="min-w-0"><span className="block break-all font-mono text-xs">{row.expected || '—'}</span>{serverOf(row) && <span className="block text-xs text-fg-muted">{serverOf(row)}</span>}</span>
                    <span role="cell" className={`min-w-0 break-words ${stateClass(row.state)}`}>{rowStateText(t, row)}</span>
                </div>)}
            </div>}
        {where && <p className="text-sm leading-6 text-fg-muted">{t(`dnsov.names.where.${where}`, { panel: mainPanel })}</p>}
        {value.home === 'own' && <p className="text-sm leading-6 text-fg-muted">{t('dnsov.names.locked')}</p>}
    </section>;
}

type MoveState =
    | { step: 'idle' }
    | { step: 'reading' }
    | { step: 'confirm'; record: DNSRecordRow }
    | { step: 'sending'; record: DNSRecordRow }
    | { step: 'done' }
    | { step: 'saved' }
    | { step: 'problem'; text: string };

/** A TTL in the owner's words: seconds, minutes or hours. */
function ttlText(t: T, seconds: number): string {
    if (seconds === 3600) return t('dnsov.ttl.oneHour');
    if (seconds === 60) return t('dnsov.ttl.oneMinute');
    if (seconds > 3600 && seconds % 3600 === 0) return t('dnsov.ttl.hours', { n: seconds / 3600 });
    if (seconds > 60) return t('dnsov.ttl.minutes', { n: Math.round(seconds / 60) });
    return t('dnsov.ttl.seconds', { n: seconds });
}

function MailCard({ value, t, rereadOverview }: { value: Overview; t: T; rereadOverview: () => Promise<Remote<Overview>> }) {
    const i18n = useI18n();
    const [move, setMove] = useState<MoveState>({ step: 'idle' });
    const inFlight = useRef(false);
    // A lost answer is never sent again: the overview is read again, and the
    // button stays off until that read answered.
    const answer = useLostAnswer(rereadOverview);
    const receiver = value.mail_receiver;
    const receiverServer = value.servers.find(server => server.who === receiver.server);
    const receiverLabel = dnsOverviewServerLabel(t, receiverServer || { who: receiver.server === 'this' ? 'this' : 'peer', short_name: receiver.short_name });
    const api = value.own_records_link.api;
    const offer = value.mx_move && api ? value.mx_move : null;
    const target = offer ? dnsOverviewServerLabel(t, value.servers.find(server => server.who === offer.to) || { who: offer.to }) : '';
    // Read the list first (only reads): exactly one MX at the apex is
    // changed, and the confirmation names its real TTL.
    async function prepare() {
        if (!offer || !api || inFlight.current || answer.holding) return;
        inFlight.current = true;
        setMove({ step: 'reading' });
        try {
            const listed = await readRemote(`${api}/records`, decodeDNSRecordRows);
            if (listed.state !== 'known') { setMove({ step: 'problem', text: t('dnsov.mail.readFailed') }); return; }
            const found = dnsOverviewMXTarget(listed.value, offer.zone, offer.to_host);
            if ('none' in found) {
                if (found.none === 'done') { setMove({ step: 'done' }); void rereadOverview(); return; }
                setMove({ step: 'problem', text: found.none === 'many' ? t('dnsov.mail.manyMX', { domain: value.domain, count: found.count }) : t('dnsov.mail.noMX') });
                return;
            }
            setMove({ step: 'confirm', record: found.record });
        } finally {
            inFlight.current = false;
        }
    }
    // The one change, sent once, with the content it was read with: a record
    // changed meanwhile is refused by the server, never overwritten.
    async function moveMail(record: DNSRecordRow) {
        if (!offer || !api || inFlight.current || answer.holding) return;
        inFlight.current = true;
        setMove({ step: 'sending', record });
        try {
            const res = await answer.send(`${api}/records`, {
                method: 'PUT', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ id: record.id, content: offer.to_host, ttl: record.ttl, prio: record.prio ?? 10, disabled: false, expected_content: record.content }),
            });
            if (!res) { setMove({ step: 'problem', text: t('dnsov.mail.lost') }); return; }
            if (!res.ok) {
                const refusal = await readApiError(res);
                // Saved in the list, not yet published: a known state of its own.
                if (refusal.code === 'DNS_PUBLICATION_FAILED') { setMove({ step: 'saved' }); void rereadOverview(); return; }
                setMove({ step: 'problem', text: t('dnsov.mail.refused', { reason: apiErrorText(refusal, i18n.t, 'common.error') }) });
                return;
            }
            answer.settle();
            setMove({ step: 'done' });
            void rereadOverview();
        } finally {
            inFlight.current = false;
        }
    }
    const confirming = move.step === 'confirm' || move.step === 'sending' ? move.record : null;
    return <section aria-labelledby="dns-mail-title" className="space-y-3 rounded-lg border border-border bg-surface p-4 sm:p-5" data-dns-mail="">
        <h2 id="dns-mail-title" className="font-semibold">{t('dnsov.mail.title')}</h2>
        {receiver.host && <p className="text-sm leading-6">{t('dnsov.mail.receiver', { domain: value.domain, server: receiverLabel, host: receiver.host })}</p>}
        <ul className="space-y-3 text-sm">{value.mail.map(row => <MailRow key={row.kind} row={row} t={t} peerKey={row.kind === 'dkim' && receiver.server === 'peer'} />)}</ul>
        {offer && move.step !== 'done' && move.step !== 'saved' && <div className="space-y-3">
            {!confirming
                ? <Button type="button" variant="primary" disabled={answer.holding || move.step === 'reading'} loading={move.step === 'reading'} onClick={() => void prepare()}>{t('dnsov.mail.move', { server: target })}</Button>
                : <div role="group" aria-labelledby="dns-mail-confirm-title" className="space-y-3 rounded-md border border-warning-mark/50 bg-warning-mark/10 p-4" data-dns-mail-confirm="">
                    <h3 id="dns-mail-confirm-title" className="font-semibold">{t('dnsov.mail.confirmTitle', { domain: value.domain, server: target })}</h3>
                    <p className="text-sm leading-6">{t('dnsov.mail.confirmBody', { domain: value.domain, server: target, host: offer.to_host, old: confirming.content, ttl: ttlText(t, confirming.ttl) })}</p>
                    <div className="flex flex-wrap gap-3">
                        <Button type="button" variant="primary" disabled={move.step === 'sending'} loading={move.step === 'sending'} onClick={() => void moveMail(confirming)}>{t(move.step === 'sending' ? 'dnsov.mail.moving' : 'dnsov.mail.confirm')}</Button>
                        <Button type="button" variant="secondary" disabled={move.step === 'sending'} onClick={() => setMove({ step: 'idle' })}>{t('dnsov.mail.cancel')}</Button>
                    </div>
                </div>}
            {move.step === 'problem' && <p role="alert" className="text-sm leading-6" data-dns-mail-problem="">{move.text}</p>}
        </div>}
        {move.step === 'done' && <p role="status" className="text-sm leading-6 text-success" data-dns-mail-moved="">{t('dnsov.mail.moved', { host: receiver.host || '' })}</p>}
        {move.step === 'saved' && <p role="status" className="text-sm leading-6" data-dns-mail-saved="">{t('dnsov.mail.savedNotPublished', { host: receiver.host || '' })}</p>}
    </section>;
}

function MailRow({ row, t, peerKey }: { row: DNSOverviewMail; t: T; peerKey: boolean }) {
    return <li className="space-y-1" data-dns-mail-row={row.kind} data-state={row.state}>
        <span className="flex flex-wrap items-baseline justify-between gap-x-3"><span className="font-medium">{t(`dnsov.mail.kind.${row.kind}`)}</span><span className={stateClass(row.state)}>{rowStateText(t, row)}</span></span>
        <span className="block break-all font-mono text-xs text-fg-muted">{row.name}{row.expected ? ` · ${t('dnsov.mail.expected', { value: row.expected })}` : ''}</span>
        {peerKey && <span className="block text-xs text-fg-muted">{t('dnsov.mail.dkimPeer')}</span>}
    </li>;
}

function OwnRecordsCard({ value, t }: { value: Overview; t: T }) {
    const own = value.own_records_link;
    return <section aria-labelledby="dns-own-title" className="space-y-2 rounded-lg border border-border bg-surface p-4 sm:p-5" data-dns-own={own.where}>
        <h2 id="dns-own-title" className="font-semibold">{t('dnsov.own.title')}</h2>
        <p className="text-sm leading-6 text-fg-muted">{t(`dnsov.own.${own.where}`, { panel: own.panel_name || '—' })}</p>
        {own.where === 'here' && own.path && <Link to={own.path} className="inline-flex text-sm font-medium text-primary underline underline-offset-4">{t('dnsov.own.open')}</Link>}
    </section>;
}
