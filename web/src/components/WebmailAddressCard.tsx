// Webmail's own address (alpha.110). Roundcube does not run on the panel's
// origin: a script running in it could call the panel's API with the signed-in
// administrator's session. Webmail gets a host name of its own (for example
// mail.example.com), answered by the server's own web server with its own
// certificate, so it keeps answering when the panel is stopped.
//
// The card says the current reason first (what is set, what is missing, what
// is running), who acts, the next action and how the work resumes. A verified
// failure, an unmet prerequisite (no DNS record, a closed port) and an unknown
// result (DNS could not be asked) are three different things here. Pressing
// Check only reads; one confirmed press of Set up sends ONE request with a
// request id, and the operation is then only read (lib/webmailAddressFlow.ts).
//
// Web postasının kendi adresi: panelin origin'inde Roundcube çalışmaz. Kart
// önce mevcut nedeni, kimin işlem yapacağını, sonraki adımı ve işin nasıl
// süreceğini söyler; doğrulanmış hata, eksik önkoşul ve bilinmeyen sonuç ayrıdır.
import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Globe } from 'lucide-react';
import { useI18n } from '../i18n';
import { invalidate, useRemote } from '../lib/remote';
import { Button, Checking, Dialog, Field, inputClass, KeepWords, RemoteGate } from './ui';
import { LifeLine, lifeDate } from './LifeLine';
import { Link } from '../router';
import {
    decodeWebmailAddress,
    decodeWebmailAddressCheck,
    webmailAddressOutcome,
    WEBMAIL_ADDRESS_CHECK_URL,
    WEBMAIL_ADDRESS_REMOVE_URL,
    WEBMAIL_ADDRESS_URL,
    type WebmailAddressCheck,
    type WebmailAddressOperation,
    type WebmailAddressRead,
} from '../lib/webmailAddress';
import { sayCheck, sayOutcome } from '../lib/webmailAddressText';
import { useWebmailAddressOperation } from '../lib/webmailAddressFlow';
import { confirmAddToZone, confirmAskPeer, RECHECK_EVERY_MS, RECHECK_FOR_MS, type RecordsAdded } from '../lib/webmailAddressRecords';

const JSON_HEADERS = { 'Content-Type': 'application/json' };
const FIREWALL_PATH = '/security#firewall';

export function WebmailAddressCard() {
    const { t } = useI18n();
    const handle = useRemote(WEBMAIL_ADDRESS_URL, decodeWebmailAddress);
    return (
        <section className="rounded-2xl border border-border bg-surface" data-webmail-address="">
            <header className="flex items-center gap-2 border-b border-border px-5 py-3">
                <Globe className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                <h2 className="text-sm font-semibold text-fg">{t('life.addr.title')}</h2>
            </header>
            <div className="space-y-3 p-5">
                <RemoteGate remote={handle.remote} checking={t('life.addr.checking')} failed={t('life.addr.unknown')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value }) => <AddressFacts value={value} onChanged={() => { void handle.retry(); invalidate('components'); }} />}
                </RemoteGate>
            </div>
        </section>
    );
}

type CheckState =
    | { phase: 'idle' }
    | { phase: 'checking' }
    | { phase: 'failed' }
    | { phase: 'known'; check: WebmailAddressCheck; typed: string };

function AddressFacts({ value, onChanged }: { value: WebmailAddressRead; onChanged: () => void }) {
    const { t, locale } = useI18n();
    const { address, status } = value;
    const [editing, setEditing] = useState(false);
    const [typedHost, setTypedHost] = useState<string | null>(null);
    const [checkState, setCheckState] = useState<CheckState>({ phase: 'idle' });
    const [confirming, setConfirming] = useState<'setup' | 'remove' | null>(null);
    const [removeCertificate, setRemoveCertificate] = useState(false);
    const [pending, setPending] = useState<{ remove: boolean; host: string } | null>(null);
    const [copied, setCopied] = useState<'records' | 'optional' | null>(null);
    // The record add (lib/webmailAddressRecords.ts): the check it was tried
    // for (no second offer for that answer), what a successful one left, and
    // the bounded wait for public DNS that follows it.
    const [tried, setTried] = useState<WebmailAddressCheck | null>(null);
    const [added, setAdded] = useState<RecordsAdded | null>(null);
    const [waiting, setWaiting] = useState<{ host: string; since: number } | null>(null);
    const [waitOver, setWaitOver] = useState(false);
    const checking = useRef(false);
    const alive = useRef(true);
    useEffect(() => () => { alive.current = false; }, []);

    const flow = useWebmailAddressOperation({
        lastOperation: value.lastOperation,
        onChanged,
        onNotReady: (check) => setCheckState({ phase: 'known', check, typed: check.host }),
    });
    const shown: WebmailAddressOperation | null = flow.operation ?? value.lastOperation;
    const working = flow.following || flow.starting || (shown !== null && (shown.status === 'queued' || shown.status === 'running'));
    const needsAgain = address !== null && status !== null && !status.present;
    const formOpen = address === null || editing || needsAgain;
    const defaultHost = needsAgain && address ? address.host : address && value.suggestion === address.host ? '' : value.suggestion;
    const host = typedHost ?? defaultHost;

    // The set-up that was followed to success closes the form and the check.
    useEffect(() => {
        if (flow.operation && flow.operation.kind === 'webmail_address' && flow.operation.status === 'succeeded') {
            setEditing(false);
            setTypedHost(null);
            setCheckState({ phase: 'idle' });
        }
    }, [flow.operation?.id, flow.operation?.status]);

    // One read-only check. Its answer, or null when it was not run (another
    // check is running) or could not be read. A quiet one (the wait after an
    // add) keeps the last answer on screen while it reads.
    const runCheck = async (name: string, quiet = false): Promise<WebmailAddressCheck | null> => {
        if (checking.current) return null;
        checking.current = true;
        if (!quiet) setCheckState({ phase: 'checking' });
        try {
            let res: Response | null = null;
            try {
                res = await fetch(WEBMAIL_ADDRESS_CHECK_URL, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify({ host: name }) });
            } catch {
                res = null;
            }
            let check: WebmailAddressCheck | null = null;
            if (res && res.ok) {
                try {
                    check = decodeWebmailAddressCheck(await res.json());
                } catch {
                    check = null;
                }
            }
            if (!alive.current) return null;
            if (check) setCheckState({ phase: 'known', check, typed: name });
            else if (!quiet) setCheckState({ phase: 'failed' });
            return check;
        } finally {
            checking.current = false;
        }
    };

    const onSubmitCheck = (event: FormEvent) => {
        event.preventDefault();
        const name = host.trim();
        if (name === '' || working) return;
        if (waiting && waiting.host !== name) setWaiting(null);
        void runCheck(name);
    };

    // After an add: public DNS is read again by itself every RECHECK_EVERY_MS,
    // for at most RECHECK_FOR_MS, with visible waiting. Each pass is the same
    // read-only check as the button; it never sends a change.
    // Eklemeden sonra genel DNS sınırlı aralıklarla yalnız okunur.
    useEffect(() => {
        if (!waiting) return;
        let timer: ReturnType<typeof setTimeout> | undefined;
        let stopped = false;
        const pass = async () => {
            const seen = await runCheck(waiting.host, true);
            if (!alive.current || stopped) return;
            if (seen && seen.state !== 'not_found') {
                setWaiting(null);
            } else if (Date.now() - waiting.since >= RECHECK_FOR_MS) {
                setWaiting(null);
                setWaitOver(true);
            } else {
                timer = setTimeout(() => void pass(), RECHECK_EVERY_MS);
            }
        };
        timer = setTimeout(() => void pass(), RECHECK_EVERY_MS);
        return () => {
            stopped = true;
            clearTimeout(timer);
        };
    }, [waiting]);

    const addHooks = (name: string) => ({
        t,
        check: () => void runCheck(name),
        onAdded: (done: RecordsAdded) => {
            if (!alive.current) return;
            setAdded(done);
            setWaitOver(false);
            setWaiting({ host: done.host, since: Date.now() });
        },
    });
    // The offer only opens the panel's own confirmation; nothing is sent before it.
    const addToZone = async (check: WebmailAddressCheck) => {
        if (!(await confirmAddToZone(check, addHooks(check.host)))) return;
        if (alive.current) setTried(check);
        // Records may have been added even when a later one failed: the zone's
        // record list and the DNS overview read again wherever they are shown.
        // Kayıtlar sonraki biri başarısız olsa da eklenmiş olabilir.
        invalidate('dns');
    };
    const askPeer = async (check: WebmailAddressCheck) => {
        if ((await confirmAskPeer(check, addHooks(check.host))) && alive.current) setTried(check);
    };

    const copy = (text: string, which: 'records' | 'optional') => {
        try {
            void navigator.clipboard?.writeText(text).then(() => {
                setCopied(which);
                setTimeout(() => { if (alive.current) setCopied(null); }, 2000);
            }, () => undefined);
        } catch {
            /* the text is selectable */
        }
    };

    const confirmSetup = (check: WebmailAddressCheck) => {
        setPending({ remove: false, host: check.host });
        void flow.start(WEBMAIL_ADDRESS_URL, { host: check.host }).then(() => { if (alive.current) setConfirming(null); });
    };
    const confirmRemove = () => {
        if (!address) return;
        setPending({ remove: true, host: address.host });
        void flow.start(WEBMAIL_ADDRESS_REMOVE_URL, { host: address.host, remove_certificate: removeCertificate }).then(() => { if (alive.current) setConfirming(null); });
    };

    // What the operation (followed here, or the last one the server holds)
    // says. A failure stays said until the address is what it aimed at; a
    // success is said where it was followed.
    let outcomeLine: ReturnType<typeof sayOutcome> | null = null;
    if (shown && !working && !flow.notice) {
        const done = webmailAddressOutcome(shown);
        const own = flow.operation !== null;
        const matters = done.remove
            ? (done.outcome === 'removed' ? own : address?.host === done.host)
            : (done.outcome === 'enabled' ? own : address?.host !== done.host);
        if (matters) outcomeLine = sayOutcome(done, t, (date) => lifeDate(date, locale));
    }
    const workingHost = shown && (shown.status === 'queued' || shown.status === 'running') ? shown.package_name ?? '' : pending?.host ?? '';
    const workingRemove = shown && (shown.status === 'queued' || shown.status === 'running') ? shown.kind === 'webmail_address_remove' : pending?.remove === true;

    const known = checkState.phase === 'known' ? checkState : null;
    const check = known?.check ?? null;
    const fresh = known !== null && known.typed === host.trim();
    const said = check ? sayCheck(check, t) : null;
    // DNS was asked only for a name that got as far as the lookup.
    const asked = check !== null && (check.state === 'ready' || check.state === 'not_found' || check.state === 'elsewhere');
    // A record added for this name is never offered again from here (the
    // wait, or Check, shows what DNS holds); a tried answer needs a new Check.
    const addedHere = check !== null && added !== null && added.host === check.host;
    const offerAdd = check !== null && check !== tried && !addedHere;

    return (
        <div className="space-y-3" data-webmail-address-state={address ? 'enabled' : 'none'}>
            {working && (
                <LifeLine tone="info" attr="webmail-address-working">
                    {t(workingRemove ? 'life.addr.removing' : 'life.addr.working', { host: workingHost })}
                </LifeLine>
            )}
            {flow.notice && (
                <LifeLine tone={flow.notice.tone} attr="webmail-address-notice">
                    <KeepWords text={t(flow.notice.key, { ...flow.notice.vars, ...(flow.notice.reasonKey ? { reason: t(flow.notice.reasonKey) } : {}) })} />
                </LifeLine>
            )}
            {outcomeLine && (
                <>
                    <LifeLine tone={outcomeLine.tone} attr="webmail-address-result"><KeepWords text={outcomeLine.text} /></LifeLine>
                    {outcomeLine.extras.map((extra) => <LifeLine key={extra.text} tone={extra.tone} attr="webmail-address-extra"><KeepWords text={extra.text} /></LifeLine>)}
                </>
            )}

            {address ? (
                <div className="space-y-2" data-webmail-address-set="">
                    <p className="text-sm text-fg">{t('life.addr.opensAt')}</p>
                    {needsAgain ? (
                        <p className="break-all text-sm font-medium text-fg">{address.url}</p>
                    ) : (
                        <a
                            href={address.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-block max-w-full break-all text-sm font-medium text-primary underline underline-offset-2"
                            data-webmail-address-link=""
                        >
                            {address.url}
                        </a>
                    )}
                    <p className="text-sm text-fg-muted">{t('life.addr.signIn')}</p>
                    {value.statusUnknown || status === null ? (
                        <LifeLine tone="info" attr="webmail-address-status-unknown">{t('life.addr.statusUnknown')}</LifeLine>
                    ) : status.error ? (
                        <>
                            <LifeLine tone="info" attr="webmail-address-status-unknown">{t('life.addr.statusUnknown')}</LifeLine>
                            <p className="pl-6 text-sm text-fg-subtle"><KeepWords text={t('life.addr.notProvedReason', { reason: status.error })} /></p>
                        </>
                    ) : !status.present ? (
                        <LifeLine tone="warn" attr="webmail-address-not-present">{t('life.addr.notPresent')}</LifeLine>
                    ) : (
                        <>
                            {!status.managed && <LifeLine tone="warn" attr="webmail-address-not-managed">{t('life.addr.notManaged')}</LifeLine>}
                            {status.certificateExpires
                                ? <LifeLine tone="ok" attr="webmail-address-certificate">{t('life.addr.certUntil', { date: lifeDate(status.certificateExpires, locale) })}</LifeLine>
                                : <LifeLine tone="info" attr="webmail-address-certificate">{t('life.addr.certUnknown')}</LifeLine>}
                            {status.proved
                                ? <LifeLine tone="ok" attr="webmail-address-proved">{t('life.addr.proved')}</LifeLine>
                                : <LifeLine tone="warn" attr="webmail-address-not-proved">{t('life.addr.notProved')}</LifeLine>}
                            {!status.proved && status.proofError && (
                                <p className="pl-6 text-sm text-fg-subtle"><KeepWords text={t('life.addr.notProvedReason', { reason: status.proofError })} /></p>
                            )}
                        </>
                    )}
                    {!formOpen && (
                        <div className="flex flex-wrap gap-2 pt-1">
                            <Button onClick={() => setEditing(true)} disabled={working} className="max-sm:min-h-11" data-webmail-address-change="">{t('life.addr.change')}</Button>
                            <Button onClick={() => { setRemoveCertificate(false); setConfirming('remove'); }} disabled={working} className="max-sm:min-h-11" data-webmail-remove-offer="">{t('life.addr.remove')}</Button>
                        </div>
                    )}
                </div>
            ) : (
                <>
                    <p className="text-sm text-fg">{t('life.addr.noneYet')}</p>
                    <p className="text-sm text-fg-muted">{t('life.addr.why')}</p>
                </>
            )}

            {formOpen && (
                <form onSubmit={onSubmitCheck} className="space-y-3" data-webmail-address-form="">
                    {address && <p className="text-sm text-fg-muted">{t('life.addr.why')}</p>}
                    <Field label={t('life.addr.hostLabel')} hint={t('life.addr.hostHint')} htmlFor="webmail-address-host">
                        <input
                            id="webmail-address-host"
                            type="text"
                            className={`${inputClass} field-lg`}
                            value={host}
                            onChange={(event) => setTypedHost(event.target.value)}
                            placeholder="mail.example.com"
                            autoCapitalize="none"
                            autoCorrect="off"
                            spellCheck={false}
                            inputMode="url"
                            maxLength={253}
                            disabled={working}
                            data-webmail-host=""
                        />
                    </Field>
                    <div className="flex flex-wrap items-center gap-2">
                        <Button type="submit" variant="primary" loading={checkState.phase === 'checking'} disabled={working || host.trim() === '' || checkState.phase === 'checking'} className="max-sm:min-h-11" data-webmail-check="">
                            {t('life.addr.check')}
                        </Button>
                        {address && editing && !needsAgain && (
                            <Button type="button" onClick={() => { setEditing(false); setTypedHost(null); setCheckState({ phase: 'idle' }); }} disabled={working} className="max-sm:min-h-11">{t('common.cancel')}</Button>
                        )}
                    </div>

                    {checkState.phase === 'checking' && <Checking label={t('life.addr.checkingNow')} />}
                    {checkState.phase === 'failed' && <LifeLine tone="info" attr="webmail-address-check-failed">{t('life.addr.checkFailed')}</LifeLine>}
                    {known && said && check && (
                        <div className="space-y-2 rounded-lg border border-border p-3" data-webmail-check-result={check.state} data-webmail-check-kind={said.kind}>
                            <p className="text-xs font-semibold uppercase tracking-wide text-fg-subtle">{said.kindText}</p>
                            <LifeLine tone={said.tone} attr="webmail-address-check-message"><KeepWords text={said.headline} /></LifeLine>
                            {/* After an add for this name, the wait (or its end) is the next step, said below. */}
                            {(said.who || said.next) && !addedHere && (
                                <p className="pl-6 text-sm leading-6 text-fg">
                                    {said.who && <span className="block">{t('life.addr.who', { who: said.who })}</span>}
                                    {said.next && <span className="block"><KeepWords text={t('life.addr.next', { next: said.next })} /></span>}
                                </p>
                            )}
                            {!fresh && <LifeLine tone="info" attr="webmail-address-check-stale">{t('life.addr.stale')}</LifeLine>}
                            <p className="pl-6 text-xs text-fg-muted break-all">
                                {asked && (check.answered.length > 0 ? t('life.addr.dnsAnswered', { answered: check.answered.join(', ') }) : t('life.addr.dnsNone'))}
                                {check.expected.length > 0 && <>{asked && <br />}{t('life.addr.thisServer', { expected: check.expected.join(', ') })}</>}
                            </p>
                            {check.state === 'not_found' && check.records.length > 0 && (
                                <div className="space-y-2 pl-6">
                                    <p className="text-sm text-fg">{t('life.addr.recordIntro')}</p>
                                    <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-lg border border-border bg-surface-2 p-3 font-mono text-xs text-fg" data-webmail-records="">{check.records.map((record) => record.text).join('\n')}</pre>
                                    <div className="flex flex-wrap gap-2">
                                        <Button type="button" onClick={() => copy(check.records.map((record) => record.text).join('\n'), 'records')} className="max-sm:min-h-11">
                                            {copied === 'records' ? t('life.addr.copied') : t('life.addr.copy')}
                                        </Button>
                                        {offerAdd && check.zone && (
                                            <Button type="button" variant="primary" onClick={() => void addToZone(check)} disabled={working || !fresh} className="max-sm:min-h-11" data-webmail-zone-offer="">
                                                {t('life.addr.addToZone', { zone: check.zone.name })}
                                            </Button>
                                        )}
                                        {offerAdd && !check.zone && check.peerZone && (
                                            <Button type="button" variant="primary" onClick={() => void askPeer(check)} disabled={working || !fresh} className="max-sm:min-h-11" data-webmail-peer-offer="">
                                                {t('dnsadd.addPeer')}
                                            </Button>
                                        )}
                                    </div>
                                    {!check.zone && !check.peerZone && check.managedBy && (
                                        <p className="text-sm text-fg-muted break-all" data-webmail-managed-by="">
                                            {t('dnsadd.managedBy', { zone: check.managedBy.zone, nameservers: check.managedBy.nameservers.join(', ') })}
                                        </p>
                                    )}
                                </div>
                            )}
                            {addedHere && waiting && <Checking label={t('dnsadd.wait')} />}
                            {addedHere && !waiting && waitOver && check.state === 'not_found' && (
                                <LifeLine tone="info" attr="webmail-address-wait-over">
                                    {added.server ? t('dnsadd.waitOver.peer', { server: added.server }) : t('dnsadd.waitOver.zone', { zone: added.zone })}
                                </LifeLine>
                            )}
                            {addedHere && added.optional && (
                                <div className="space-y-2 pl-6" data-webmail-optional-record="">
                                    <p className="text-sm text-fg-muted">{added.optional.why}</p>
                                    <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-lg border border-border bg-surface-2 p-3 font-mono text-xs text-fg">{added.optional.text}</pre>
                                    <Button type="button" onClick={() => copy(added.optional!.text, 'optional')} className="max-sm:min-h-11">
                                        {copied === 'optional' ? t('life.addr.copied') : t('life.addr.copy')}
                                    </Button>
                                </div>
                            )}
                            {check.firewall.state === 'closed' && (
                                <p className="pl-6 text-sm">
                                    <Link to={FIREWALL_PATH} className="font-medium text-primary underline underline-offset-2" data-webmail-firewall-link="">{t('life.addr.fw.link')}</Link>
                                </p>
                            )}
                            {asked && check.firewall.state === 'unknown' && <LifeLine tone="info" attr="webmail-address-firewall-unknown">{t('life.addr.fw.unknown')}</LifeLine>}
                            {check.warnings.filter((warning) => !/firewall/i.test(warning)).map((warning) => <LifeLine key={warning} tone="info" attr="webmail-address-warning">{warning}</LifeLine>)}
                            {check.ready && (
                                <div className="pl-6 pt-1">
                                    <Button type="button" variant="primary" onClick={() => setConfirming('setup')} disabled={working || !fresh} className="max-sm:min-h-11" data-webmail-setup-offer="">
                                        {t('life.addr.setup', { host: check.host })}
                                    </Button>
                                </div>
                            )}
                        </div>
                    )}
                </form>
            )}

            {confirming === 'setup' && check && (
                <Dialog
                    id="webmail-address-setup"
                    title={t('life.addr.setupTitle', { host: check.host })}
                    busy={flow.starting}
                    width="lg"
                    onDismiss={() => setConfirming(null)}
                    actions={
                        <>
                            <Button onClick={() => setConfirming(null)} disabled={flow.starting}>{t('common.cancel')}</Button>
                            <Button variant="primary" onClick={() => confirmSetup(check)} loading={flow.starting} disabled={flow.starting || working} data-webmail-setup-confirm="">
                                {t('life.addr.setupConfirm')}
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm leading-6 text-fg">{t('life.addr.setupIntro')}</p>
                    <ol className="mt-1 list-decimal space-y-1 pl-5 text-sm leading-6 text-fg">
                        <li>{t('life.addr.step.cert', { host: check.host })}</li>
                        <li>{t('life.addr.step.nginx', { host: check.host })}</li>
                        <li>{t('life.addr.step.prove')}</li>
                        {check.current !== '' && check.current !== check.host && <li>{t('life.addr.step.replace', { current: check.current })}</li>}
                    </ol>
                    <p className="mt-2 text-sm leading-6 text-fg">{t('life.addr.setupSafe')}</p>
                    <p className="mt-2 text-sm leading-6 text-fg-muted">{t('life.addr.setupLeave')}</p>
                </Dialog>
            )}

            {confirming === 'remove' && address && (
                <Dialog
                    id="webmail-address-remove"
                    title={t('life.addr.removeTitle', { host: address.host })}
                    tone="danger"
                    busy={flow.starting}
                    width="lg"
                    onDismiss={() => setConfirming(null)}
                    actions={
                        <>
                            <Button onClick={() => setConfirming(null)} disabled={flow.starting}>{t('common.cancel')}</Button>
                            <Button variant="danger" onClick={confirmRemove} loading={flow.starting} disabled={flow.starting || working} data-webmail-remove-confirm="">
                                {t('life.addr.removeConfirm')}
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm leading-6 text-fg">{t('life.addr.removeBody', { host: address.host })}</p>
                    <label className="mt-3 flex items-start gap-2 text-sm text-fg">
                        <input
                            type="checkbox"
                            className="mt-1 h-4 w-4"
                            checked={removeCertificate}
                            onChange={(event) => setRemoveCertificate(event.target.checked)}
                            disabled={flow.starting}
                            data-webmail-remove-certificate=""
                        />
                        <span>{t('life.addr.removeCert')}</span>
                    </label>
                    <p className="mt-1 pl-6 text-xs text-fg-muted">{t('life.addr.removeCertHint')}</p>
                </Dialog>
            )}
        </div>
    );
}
