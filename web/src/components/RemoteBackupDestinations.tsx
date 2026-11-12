import { useEffect, useRef, useState } from 'react';
import {
    CalendarClock, Check, Cloud, Eye, EyeOff, Fingerprint, Pencil, Plus, Server, ShieldAlert, Trash2, UploadCloud, Wand2, X,
} from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { readApiError, type ApiError } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { useLostAnswer, type LostAnswerHandle } from '../lib/lostAnswer';
import { Button, Dialog, KnownEmpty, RemoteGate, ResultUnknown, inputClass } from './ui';
import { PasswordInput } from './PasswordInput';
import {
    REMOTE_DESTINATIONS_URL, REMOTE_DESTINATION_TEST_URL, REMOTE_POLICIES_URL, decodeDestinations, decodePolicies,
    decodeTestResult, generatePassphrase,
    type RemoteDestination, type RemotePolicy, type RemoteTestResult,
} from '../lib/remoteBackup';
import {
    AddRefusedNote, GuidanceBox, PolicyLine, RemoteHelp, RemoteRefusal, ScheduleEditor, formatServerTime, sayOrNull,
    type ScheduleBody,
} from './RemoteBackupShared';

// Remote backup destinations (alpha.118, W4): storage the owner controls
// elsewhere, S3-compatible or SFTP. The administrator's server-wide ones live
// in Settings → Remote backups, with the every-site schedules; a reseller's or
// customer's own ones in My account, when the right "Own remote backup
// destinations" is theirs (D-042). The server decides who may do what and says
// it (can_add, add_refused_by); this screen only shows it.
//
// Secrets and the passphrase are typed here and sent once; the server never
// sends them back (has_secret, passphrase_check). An SFTP server's key is
// confirmed by a person comparing its fingerprint, never accepted by itself.
//
// Uzak yedek hedefleri: sahibin başka yerde denetlediği depolama. Yönetici
// sunucu geneli hedefleri Ayarlar'da, bayi ve müşteri kendi hedeflerini
// Hesabım'da yönetir. Gizli bilgiler bir kez gönderilir, geri gelmez. SFTP
// sunucu anahtarı yalnız kişi parmak izini karşılaştırıp onaylayınca kabul edilir.

const JSON_HEADERS = { 'Content-Type': 'application/json' };

// Examples of S3-compatible endpoints; a click fills endpoint and region.
// Brand names and addresses are not translated.
const S3_EXAMPLES = [
    { name: 'Amazon S3', endpoint: 'https://s3.eu-central-1.amazonaws.com', region: 'eu-central-1' },
    { name: 'Backblaze B2', endpoint: 'https://s3.eu-central-003.backblazeb2.com', region: 'eu-central-003' },
    { name: 'Wasabi', endpoint: 'https://s3.eu-central-1.wasabisys.com', region: 'eu-central-1' },
    { name: 'MinIO', endpoint: 'https://minio.example.com:9000', region: 'us-east-1' },
    { name: 'Cloudflare R2', endpoint: 'https://ACCOUNT-ID.r2.cloudflarestorage.com', region: 'auto' },
] as const;

const HOST_KEY_COMMAND = 'ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub';

async function readJSON(res: Response): Promise<unknown> {
    try {
        return await res.json();
    } catch {
        return null;
    }
}

export function RemoteBackupDestinations({ admin }: { admin: boolean }) {
    const { t } = useI18n();
    const destinations = useRemote(REMOTE_DESTINATIONS_URL, decodeDestinations);
    const answer = useLostAnswer(() => destinations.retry());
    const [editing, setEditing] = useState<{ destination: RemoteDestination | null } | null>(null);
    const [removing, setRemoving] = useState<RemoteDestination | null>(null);
    const known = destinations.remote.state === 'known' && !destinations.reading && !answer.holding;

    const changed = () => {
        void destinations.retry();
        invalidate('remoteBackups');
    };

    return (
        <section className="space-y-4" aria-labelledby="rbak-destinations-title" aria-busy={destinations.reading}>
            <div>
                <h2 id="rbak-destinations-title" className="flex items-center gap-2 text-base font-semibold text-fg">
                    <UploadCloud className="h-5 w-5 text-primary" aria-hidden="true" />
                    {t('rbak.title')}
                </h2>
                <p className="mt-1 max-w-[75ch] text-sm text-fg-muted">{t(admin ? 'rbak.intro.server' : 'rbak.intro.own')}</p>
            </div>

            <ResultUnknown answer={answer} />

            <RemoteGate
                remote={destinations.remote}
                checking={t('rbak.checking')}
                failed={t('rbak.unknown')}
                onRetry={() => void destinations.retry()}
                busy={destinations.reading}
            >
                {(shown) => {
                    const value = shown.value;
                    const manageable = value.destinations.filter((d) => d.manageable);
                    const offered = value.destinations.filter((d) => !d.manageable);
                    const serverWide = manageable.filter((d) => d.scope === 'server');
                    return (
                        <div className="space-y-4">
                            {value.canAdd ? (
                                <div>
                                    <Button variant="primary" icon={Plus} disabled={!known || shown.stale}
                                        className="max-sm:min-h-11" onClick={() => setEditing({ destination: null })}>
                                        {t('rbak.add')}
                                    </Button>
                                </div>
                            ) : (
                                <AddRefusedNote layer={value.addRefusedBy} />
                            )}

                            {manageable.length === 0 && value.canAdd ? (
                                <KnownEmpty of={shown} icon={Cloud} title={t('rbak.empty')} hint={t('rbak.emptyHint')} />
                            ) : manageable.length > 0 && (
                                <ul className="space-y-2">
                                    {manageable.map((d) => (
                                        <DestinationRow key={d.id} destination={d} admin={admin} disabled={!known || shown.stale}
                                            onEdit={() => setEditing({ destination: d })} onRemove={() => setRemoving(d)} />
                                    ))}
                                </ul>
                            )}

                            {offered.length > 0 && (
                                <div className="rounded-xl border border-border bg-surface p-4" data-offered-destinations="">
                                    <p className="text-sm text-fg">{t('rbak.offeredByAdmin')}</p>
                                    <ul className="mt-2 flex flex-wrap gap-2">
                                        {offered.map((d) => (
                                            <li key={d.id} className="rounded-md bg-surface-2 px-2 py-1 text-xs text-fg">
                                                {d.name} · {t(d.kind === 'sftp' ? 'rbak.kind.sftp' : 'rbak.kind.s3')}
                                            </li>
                                        ))}
                                    </ul>
                                </div>
                            )}

                            {admin && <ServerPolicies destinations={serverWide} />}

                            {editing && (
                                <DestinationEditor
                                    destination={editing.destination}
                                    admin={admin}
                                    minPassphrase={value.minPassphrase}
                                    answer={answer}
                                    onClose={() => setEditing(null)}
                                    onSaved={() => { setEditing(null); changed(); }}
                                />
                            )}
                            {removing && (
                                <RemoveDialog destination={removing} answer={answer}
                                    onClose={() => setRemoving(null)} onRemoved={() => { setRemoving(null); changed(); }} />
                            )}
                        </div>
                    );
                }}
            </RemoteGate>

            <RemoteHelp />
        </section>
    );
}

function DestinationRow({ destination: d, admin, disabled, onEdit, onRemove }: {
    destination: RemoteDestination;
    admin: boolean;
    disabled: boolean;
    onEdit: () => void;
    onRemove: () => void;
}) {
    const { t, locale } = useI18n();
    const Icon = d.kind === 'sftp' ? Server : Cloud;
    const tested = formatServerTime(d.lastTestAt, locale);
    const lastTest = !d.lastTestResult
        ? t('rbak.lastTest.none')
        : d.lastTestResult === 'ok'
            ? t('rbak.lastTest.ok', { time: tested })
            : t('rbak.lastTest.failed', { time: tested, what: sayOrNull(t, `rbak.code.${d.lastTestResult}.what`) ?? d.lastTestResult });
    return (
        <li className="flex flex-wrap items-start justify-between gap-3 rounded-xl border border-border bg-surface p-4" data-destination={d.id}>
            <div className="flex min-w-0 items-start gap-3">
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <Icon className="h-4 w-4" aria-hidden="true" />
                </span>
                <div className="min-w-0">
                    <p className="break-words text-sm font-medium text-fg">{d.name}</p>
                    {d.location && <p className="break-all font-mono text-xs text-fg-muted">{d.location}</p>}
                    <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-fg-muted">
                        <span className="rounded bg-surface-2 px-1.5 py-0.5">{t(d.kind === 'sftp' ? 'rbak.kind.sftp' : 'rbak.kind.s3')}</span>
                        <span className="rounded bg-surface-2 px-1.5 py-0.5">{sayOrNull(t, `rbak.scope.${d.scope}`) ?? d.scope}</span>
                        {admin && d.offeredToAccounts && (
                            <span className="rounded bg-primary/10 px-1.5 py-0.5 text-primary">{t('rbak.offered')}</span>
                        )}
                        {d.passphraseCheck && (
                            <span className="rounded bg-surface-2 px-1.5 py-0.5" title={t('rbak.passCheckHint')}>
                                {t('rbak.passCheck', { check: d.passphraseCheck })}
                            </span>
                        )}
                    </div>
                    {d.hostKeyFingerprint && (
                        <p className="mt-1 flex items-center gap-1 break-all text-xs text-fg-muted">
                            <Fingerprint className="h-3 w-3 shrink-0" aria-hidden="true" />
                            {t('rbak.fingerprint', { fingerprint: d.hostKeyFingerprint })}
                        </p>
                    )}
                    <p className={`mt-1 text-xs ${d.lastTestResult && d.lastTestResult !== 'ok' ? 'text-warning' : 'text-fg-subtle'}`}>{lastTest}</p>
                </div>
            </div>
            <div className="flex items-center gap-1">
                <Button icon={Pencil} disabled={disabled} onClick={onEdit} className="max-sm:min-h-11"
                    aria-label={t('rbak.editNamed', { name: d.name })}>
                    {t('rbak.edit')}
                </Button>
                <Button variant="danger" icon={Trash2} disabled={disabled} onClick={onRemove} className="max-sm:min-h-11"
                    aria-label={t('rbak.removeNamed', { name: d.name })}>
                    {t('rbak.remove')}
                </Button>
            </div>
        </li>
    );
}

// ----- the add / change form -----

type Kind = 's3' | 'sftp';

function DestinationEditor({ destination, admin, minPassphrase, answer, onClose, onSaved }: {
    destination: RemoteDestination | null;
    admin: boolean;
    minPassphrase: number;
    answer: LostAnswerHandle;
    onClose: () => void;
    onSaved: () => void;
}) {
    const { t } = useI18n();
    const creating = destination === null;
    const cfg = destination?.config ?? {};
    const [name, setName] = useState(destination?.name ?? '');
    const [kind, setKind] = useState<Kind>(destination?.kind === 'sftp' ? 'sftp' : 's3');
    const [endpoint, setEndpoint] = useState(cfg.endpoint ?? '');
    const [region, setRegion] = useState(cfg.region ?? '');
    const [bucket, setBucket] = useState(cfg.bucket ?? '');
    const [prefix, setPrefix] = useState(cfg.prefix ?? '');
    // A saved access key ID comes back masked; empty keeps it.
    const [accessKey, setAccessKey] = useState('');
    const [secretKey, setSecretKey] = useState('');
    const [virtualHosted, setVirtualHosted] = useState(cfg.virtual_hosted === true);
    const [host, setHost] = useState(cfg.host ?? '');
    const [port, setPort] = useState(String(cfg.port ?? 22));
    const [user, setUser] = useState(cfg.user ?? '');
    const [auth, setAuth] = useState<'password' | 'key'>('password');
    const [password, setPassword] = useState('');
    const [privateKey, setPrivateKey] = useState('');
    const [path, setPath] = useState(cfg.path ?? '');
    const [offered, setOffered] = useState(destination?.offeredToAccounts ?? false);
    const [passphrase, setPassphrase] = useState('');
    const [passphraseAgain, setPassphraseAgain] = useState('');
    const [reveal, setReveal] = useState(false);
    const [writtenDown, setWrittenDown] = useState(false);
    const [confirmedFingerprint, setConfirmedFingerprint] = useState('');
    const [test, setTest] = useState<RemoteTestResult | null>(null);
    const [testLost, setTestLost] = useState(false);
    const [hostKeyStopped, setHostKeyStopped] = useState(false);
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const [working, setWorking] = useState<'test' | 'save' | null>(null);
    const lastAction = useRef<'test' | 'save'>('test');

    // A new server address forgets a fingerprint confirmed for the old one.
    useEffect(() => { setConfirmedFingerprint(''); }, [host, port]);

    const portNumber = Number(port);
    const portOK = Number.isInteger(portNumber) && portNumber >= 1 && portNumber <= 65535;
    const secretGiven = kind === 's3' ? secretKey !== '' : auth === 'password' ? password !== '' : privateKey.trim() !== '';
    const connectionReady = kind === 's3'
        ? endpoint.trim() !== '' && bucket.trim() !== '' && (accessKey.trim() !== '' || !creating) && (secretGiven || !creating)
        : host.trim() !== '' && user.trim() !== '' && path.trim() !== '' && portOK && (secretGiven || !creating);
    const passphraseProblem = !creating ? null
        : passphrase.length < minPassphrase ? t('rbak.form.passphraseShort', { min: minPassphrase })
            : passphrase !== passphraseAgain ? t('rbak.form.passphraseMismatch')
                : !writtenDown ? t('rbak.form.writtenDownNeeded') : null;
    const nameOK = name.trim().length >= 1 && name.trim().length <= 100;
    const canSave = connectionReady && nameOK && passphraseProblem === null && !answer.holding;
    const busy = working !== null;

    const body = (forTest: boolean, fingerprint: string) => {
        const config = kind === 's3'
            ? { endpoint: endpoint.trim(), region: region.trim(), bucket: bucket.trim(), prefix: prefix.trim(), access_key_id: accessKey.trim(), virtual_hosted: virtualHosted }
            : { host: host.trim(), port: portNumber, user: user.trim(), path: path.trim() };
        const secret = kind === 's3'
            ? { secret_access_key: secretKey }
            : auth === 'password' ? { password } : { private_key: privateKey };
        return JSON.stringify({
            name: name.trim(),
            kind,
            config,
            secret,
            ...(creating && !forTest ? { passphrase } : {}),
            ...(fingerprint ? { host_key_fingerprint: fingerprint } : {}),
            offered_to_accounts: admin && offered,
            ...(forTest && destination ? { destination_id: destination.id } : {}),
        });
    };

    const runTest = async (fingerprint = confirmedFingerprint) => {
        lastAction.current = 'test';
        setWorking('test');
        setRefusal(null);
        setTestLost(false);
        setHostKeyStopped(false);
        try {
            let res: Response;
            try {
                res = await fetch(REMOTE_DESTINATION_TEST_URL, { method: 'POST', headers: JSON_HEADERS, body: body(true, fingerprint) });
            } catch {
                setTest(null);
                setTestLost(true);
                return;
            }
            if (!res.ok) {
                setTest(null);
                setRefusal(await readApiError(res));
                return;
            }
            try {
                setTest(decodeTestResult(await res.json()));
            } catch {
                setTest(null);
                setTestLost(true);
            }
        } finally {
            setWorking(null);
        }
    };

    const save = async (fingerprint = confirmedFingerprint) => {
        if (!canSave) return;
        lastAction.current = 'save';
        setWorking('save');
        setRefusal(null);
        setTestLost(false);
        setHostKeyStopped(false);
        const savedName = name.trim();
        try {
            const res = await answer.send(
                creating ? REMOTE_DESTINATIONS_URL : `${REMOTE_DESTINATIONS_URL}/${destination!.id}`,
                { method: creating ? 'POST' : 'PUT', headers: JSON_HEADERS, body: body(false, fingerprint) },
                creating ? {
                    // Saved when the list read again holds a destination of this name.
                    shows: (read) => {
                        const value = read[0]?.value as { destinations?: { name: string }[] } | undefined;
                        return value?.destinations ? value.destinations.some((d) => d.name === savedName) : null;
                    },
                    made: () => onSaved(),
                } : undefined,
            );
            if (!res) return;
            const copy = res.clone();
            const data = await readJSON(res);
            if (res.status === 409 && data && typeof data === 'object' && 'test' in data) {
                try {
                    setTest(decodeTestResult((data as { test: unknown }).test));
                } catch {
                    setTestLost(true);
                }
                return;
            }
            if (!res.ok) {
                setRefusal(await readApiError(copy));
                return;
            }
            answer.settle();
            showToast('success', t('rbak.form.saved'));
            onSaved();
        } finally {
            setWorking(null);
        }
    };

    const confirmHostKey = (fingerprint: string) => {
        setConfirmedFingerprint(fingerprint);
        setTest(null);
        void (lastAction.current === 'save' ? save(fingerprint) : runTest(fingerprint));
    };

    const generate = () => {
        const made = generatePassphrase();
        setPassphrase(made);
        setPassphraseAgain(made);
        setReveal(true);
        setWrittenDown(false);
    };

    const id = (field: string) => `rbak-form-${field}`;
    const field = (label: string, input: React.ReactNode, hint?: React.ReactNode, htmlFor?: string) => (
        <div>
            <label htmlFor={htmlFor} className="mb-1 block text-sm font-medium text-fg-muted">{label}</label>
            {input}
            {hint && <p className="mt-1 text-xs text-fg-subtle">{hint}</p>}
        </div>
    );

    return (
        <Dialog
            id="rbak-editor"
            title={creating ? t('rbak.form.addTitle') : t('rbak.form.editTitle', { name: destination!.name })}
            description={t('rbak.form.desc')}
            icon={UploadCloud}
            width="xl"
            busy={busy}
            onDismiss={onClose}
            noValidate
            onSubmit={(event) => { event.preventDefault(); void save(); }}
            footerLead={!canSave && !busy ? (
                <p id="rbak-form-blocked" className="text-sm text-fg-muted" data-blocked-reason="">
                    {passphraseProblem ?? t('rbak.form.required')}
                </p>
            ) : undefined}
            actions={
                <>
                    <Button type="button" onClick={onClose} disabled={busy}>{t('rbak.cancel')}</Button>
                    <Button type="button" icon={Check} loading={working === 'test'} disabled={busy || !connectionReady}
                        onClick={() => void runTest()}>
                        {t('rbak.test')}
                    </Button>
                    <Button type="submit" variant="primary" loading={working === 'save'} disabled={busy || !canSave}
                        aria-describedby={!canSave ? 'rbak-form-blocked' : undefined}>
                        {t('rbak.form.save')}
                    </Button>
                </>
            }
        >
            <div className="space-y-4">
                <ResultUnknown answer={answer} />
                {field(t('rbak.form.name'),
                    <input id={id('name')} className={`${inputClass} field-md`} value={name} maxLength={100} required disabled={busy}
                        onChange={(event) => setName(event.target.value)} />,
                    t('rbak.form.nameHint'), id('name'))}

                <fieldset>
                    <legend className="mb-1 block text-sm font-medium text-fg-muted">{t('rbak.form.kind')}</legend>
                    <div className="flex flex-wrap gap-2">
                        {(['s3', 'sftp'] as const).map((option) => (
                            <label key={option} className={`flex min-h-11 cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 text-sm ${kind === option ? 'border-primary bg-primary/10 text-fg' : 'border-border-strong text-fg-muted'} ${!creating ? 'cursor-not-allowed opacity-70' : ''}`}>
                                <input type="radio" name="rbak-kind" value={option} checked={kind === option} disabled={!creating || busy}
                                    className="h-4 w-4 accent-primary" onChange={() => { setKind(option); setTest(null); }} />
                                {t(option === 's3' ? 'rbak.kind.s3' : 'rbak.kind.sftp')}
                            </label>
                        ))}
                    </div>
                    {!creating && <p className="mt-1 text-xs text-fg-subtle">{t('rbak.form.kindFixed')}</p>}
                </fieldset>

                {kind === 's3' ? (
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                        <div className="sm:col-span-2">
                            {field(t('rbak.form.endpoint'),
                                <input id={id('endpoint')} className={`${inputClass} field-xl`} value={endpoint} inputMode="url" autoComplete="off" spellCheck={false}
                                    placeholder="https://s3.eu-central-1.amazonaws.com" disabled={busy} onChange={(event) => setEndpoint(event.target.value)} />,
                                <span className="block">
                                    {t('rbak.form.endpointHint')}
                                    <span className="mt-1 flex flex-wrap gap-1.5">
                                        {S3_EXAMPLES.map((example) => (
                                            <button key={example.name} type="button" disabled={busy}
                                                className="rounded-md border border-border-strong px-2 py-1 text-xs text-fg hover:bg-surface-2 max-sm:min-h-11"
                                                title={example.endpoint}
                                                onClick={() => { setEndpoint(example.endpoint); setRegion(example.region); }}>
                                                {example.name}
                                            </button>
                                        ))}
                                    </span>
                                </span>, id('endpoint'))}
                        </div>
                        {field(t('rbak.form.region'),
                            <input id={id('region')} className={`${inputClass} field-md`} value={region} autoComplete="off" spellCheck={false} disabled={busy}
                                onChange={(event) => setRegion(event.target.value)} />, t('rbak.form.regionHint'), id('region'))}
                        {field(t('rbak.form.bucket'),
                            <input id={id('bucket')} className={`${inputClass} field-lg`} value={bucket} autoComplete="off" spellCheck={false} disabled={busy}
                                onChange={(event) => setBucket(event.target.value)} />, undefined, id('bucket'))}
                        {field(t('rbak.form.prefix'),
                            <input id={id('prefix')} className={`${inputClass} field-lg`} value={prefix} autoComplete="off" spellCheck={false} disabled={busy}
                                onChange={(event) => setPrefix(event.target.value)} />, t('rbak.form.prefixHint'), id('prefix'))}
                        {field(t('rbak.form.accessKey'),
                            <input id={id('access')} className={`${inputClass} field-lg`} value={accessKey} autoComplete="off" spellCheck={false} disabled={busy}
                                onChange={(event) => setAccessKey(event.target.value)} />,
                            !creating && cfg.access_key_id ? t('rbak.form.accessKeyKeep', { masked: cfg.access_key_id }) : undefined, id('access'))}
                        <div className="sm:col-span-2">
                            {field(t('rbak.form.secretKey'),
                                <PasswordInput id={id('secret')} value={secretKey} autoComplete="new-password"
                                    spellCheck={false} disabled={busy} onChange={setSecretKey} />,
                                !creating && destination?.hasSecret ? t('rbak.form.secretKeep') : t('rbak.form.secretHint'), id('secret'))}
                        </div>
                        <label className="flex items-start gap-3 sm:col-span-2">
                            <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={virtualHosted} disabled={busy}
                                onChange={(event) => setVirtualHosted(event.target.checked)} />
                            <span>
                                <span className="block text-sm font-medium text-fg">{t('rbak.form.virtualHosted')}</span>
                                <span className="block text-xs text-fg-subtle">{t('rbak.form.virtualHostedHint')}</span>
                            </span>
                        </label>
                    </div>
                ) : (
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-[minmax(0,1fr)_7rem]">
                        {field(t('rbak.form.host'),
                            <input id={id('host')} className={`${inputClass} field-lg`} value={host} autoComplete="off" spellCheck={false} disabled={busy}
                                placeholder="backup.example.com" onChange={(event) => setHost(event.target.value)} />, undefined, id('host'))}
                        {field(t('rbak.form.port'),
                            <input id={id('port')} type="number" inputMode="numeric" min={1} max={65535} className={`${inputClass} field-xs`} value={port}
                                disabled={busy} aria-invalid={!portOK || undefined} onChange={(event) => setPort(event.target.value)} />, undefined, id('port'))}
                        <div className="sm:col-span-2">
                            {field(t('rbak.form.user'),
                                <input id={id('user')} className={`${inputClass} field-md`} value={user} autoComplete="off" spellCheck={false} disabled={busy}
                                    onChange={(event) => setUser(event.target.value)} />, undefined, id('user'))}
                        </div>
                        <fieldset className="sm:col-span-2">
                            <legend className="mb-1 block text-sm font-medium text-fg-muted">{t('rbak.form.auth')}</legend>
                            <div className="flex flex-wrap gap-2">
                                {(['password', 'key'] as const).map((option) => (
                                    <label key={option} className={`flex min-h-11 cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 text-sm ${auth === option ? 'border-primary bg-primary/10 text-fg' : 'border-border-strong text-fg-muted'}`}>
                                        <input type="radio" name="rbak-auth" value={option} checked={auth === option} disabled={busy}
                                            className="h-4 w-4 accent-primary" onChange={() => setAuth(option)} />
                                        {t(option === 'password' ? 'rbak.form.authPassword' : 'rbak.form.authKey')}
                                    </label>
                                ))}
                            </div>
                            {!creating && destination?.hasSecret && <p className="mt-1 text-xs text-fg-subtle">{t('rbak.form.sftpSecretKeep')}</p>}
                        </fieldset>
                        <div className="sm:col-span-2">
                            {auth === 'password'
                                ? field(t('rbak.form.password'),
                                    <PasswordInput id={id('password')} value={password} autoComplete="new-password"
                                        disabled={busy} onChange={setPassword} />, undefined, id('password'))
                                : field(t('rbak.form.privateKey'),
                                    <textarea id={id('key')} rows={5} className={`${inputClass} font-mono text-xs`} value={privateKey}
                                        autoComplete="off" autoCorrect="off" autoCapitalize="off" spellCheck={false} disabled={busy}
                                        data-secret="" onChange={(event) => setPrivateKey(event.target.value)} />,
                                    t('rbak.form.privateKeyHint'), id('key'))}
                        </div>
                        <div className="sm:col-span-2">
                            {field(t('rbak.form.path'),
                                <input id={id('path')} className={`${inputClass} field-xl`} value={path} autoComplete="off" spellCheck={false} disabled={busy}
                                    placeholder="/backups/celikpanel" onChange={(event) => setPath(event.target.value)} />, t('rbak.form.pathHint'), id('path'))}
                        </div>
                    </div>
                )}

                {admin && (
                    <label className="flex items-start gap-3">
                        <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={offered} disabled={busy}
                            onChange={(event) => setOffered(event.target.checked)} />
                        <span>
                            <span className="block text-sm font-medium text-fg">{t('rbak.form.offer')}</span>
                            <span className="block text-xs text-fg-subtle">{t('rbak.form.offerHint')}</span>
                        </span>
                    </label>
                )}

                {creating ? (
                    <fieldset className="space-y-3 rounded-xl border border-warning-mark/50 bg-warning-mark/20 p-4">
                        <legend className="sr-only">{t('rbak.form.passphrase')}</legend>
                        <p className="flex items-start gap-2 text-sm text-fg" id="rbak-passphrase-warning">
                            <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                            <span>{t('rbak.form.passphraseWarning')}</span>
                        </p>
                        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                            {field(t('rbak.form.passphrase'),
                                <input id={id('passphrase')} type={reveal ? 'text' : 'password'} className={`${inputClass} font-mono field-lg`}
                                    value={passphrase} autoComplete="new-password" spellCheck={false} disabled={busy}
                                    aria-describedby="rbak-passphrase-warning" onChange={(event) => { setPassphrase(event.target.value); setWrittenDown(false); }} />,
                                t('rbak.form.passphraseHint', { min: minPassphrase }), id('passphrase'))}
                            {field(t('rbak.form.passphraseAgain'),
                                <input id={id('passphrase2')} type={reveal ? 'text' : 'password'} className={`${inputClass} font-mono field-lg`}
                                    value={passphraseAgain} autoComplete="new-password" spellCheck={false} disabled={busy}
                                    onChange={(event) => setPassphraseAgain(event.target.value)} />, undefined, id('passphrase2'))}
                        </div>
                        <div className="flex flex-wrap gap-2">
                            <Button type="button" icon={Wand2} disabled={busy} onClick={generate} className="max-sm:min-h-11">{t('rbak.form.generate')}</Button>
                            <Button type="button" icon={reveal ? EyeOff : Eye} disabled={busy} onClick={() => setReveal(!reveal)}
                                aria-pressed={reveal} className="max-sm:min-h-11">
                                {t(reveal ? 'rbak.form.hide' : 'rbak.form.show')}
                            </Button>
                        </div>
                        <label className="flex items-start gap-3">
                            <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={writtenDown} disabled={busy}
                                onChange={(event) => setWrittenDown(event.target.checked)} />
                            <span className="text-sm font-medium text-fg">{t('rbak.form.writtenDown')}</span>
                        </label>
                    </fieldset>
                ) : (
                    <p className="rounded-lg border border-border bg-surface-2 p-3 text-sm text-fg-muted">
                        {t('rbak.form.passphraseFixed', { check: destination!.passphraseCheck || '—' })}
                    </p>
                )}

                <RemoteRefusal problem={refusal} sentence="rbak.form.saveFailed" />
                {testLost && (
                    <GuidanceBox context="test" guidance={{
                        result: 'unknown', code: '', reason: t('rbak.test.lost'), whoActs: 'you', nextAction: t('rbak.test.lostTodo'), resumes: '',
                    }} />
                )}
                {hostKeyStopped && <p className="text-sm text-fg" role="status">{t('rbak.hostKey.stopped')}</p>}
                {test && <TestResultView result={test} busy={busy} onConfirm={confirmHostKey}
                    onReject={() => { setTest(null); setHostKeyStopped(true); }} />}
                {confirmedFingerprint && !test?.hostKey && (
                    <p className="break-all text-xs text-fg-muted">{t('rbak.hostKey.confirmed', { fingerprint: confirmedFingerprint })}</p>
                )}
            </div>
        </Dialog>
    );
}

function TestResultView({ result, busy, onConfirm, onReject }: {
    result: RemoteTestResult;
    busy: boolean;
    onConfirm: (fingerprint: string) => void;
    onReject: () => void;
}) {
    const { t } = useI18n();
    const changed = result.guidance?.code === 'REMOTE_HOST_KEY';
    return (
        <div className="space-y-3" data-test-result={result.ok ? 'ok' : 'failed'}>
            {result.steps.length > 0 && (
                <div>
                    <p className="mb-1 text-sm font-medium text-fg">{t('rbak.test.title')}</p>
                    <ol className="grid grid-cols-1 gap-1 sm:grid-cols-2">
                        {result.steps.map((step) => (
                            <li key={step.step} className="flex items-center gap-2 text-sm text-fg">
                                {step.ok
                                    ? <Check className="h-4 w-4 shrink-0 text-success" aria-hidden="true" />
                                    : <X className="h-4 w-4 shrink-0 text-danger" aria-hidden="true" />}
                                <span>{sayOrNull(t, `rbak.test.step.${step.step}`) ?? step.step}</span>
                                <span className="sr-only">{t(step.ok ? 'rbak.test.passed' : 'rbak.test.failedStep')}</span>
                            </li>
                        ))}
                    </ol>
                </div>
            )}
            {result.ok && <p className="text-sm text-success" role="status">{t('rbak.test.ok')}</p>}
            {result.hostKey ? (
                <div className={`space-y-2 rounded-xl border p-4 ${changed ? 'border-danger/30 bg-danger/10' : 'border-warning-mark/50 bg-warning-mark/20'}`}
                    role="alert" data-host-key={changed ? 'changed' : 'new'}>
                    <p className="flex items-center gap-2 text-sm font-semibold text-fg">
                        <Fingerprint className="h-4 w-4 shrink-0" aria-hidden="true" />
                        {t(changed ? 'rbak.hostKey.titleChanged' : 'rbak.hostKey.titleNew')}
                    </p>
                    <p className="text-sm text-fg">{t('rbak.hostKey.intro')}</p>
                    <p className="break-all rounded-lg bg-surface p-3 font-mono text-sm font-semibold text-fg" data-fingerprint="">
                        {result.hostKey.fingerprint}
                    </p>
                    <p className="text-sm text-fg">{t('rbak.hostKey.howTo')}</p>
                    <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-lg bg-surface p-2 font-mono text-xs text-fg">{HOST_KEY_COMMAND}</pre>
                    <p className="text-xs text-fg-muted">{t('rbak.hostKey.otherTypes')}</p>
                    {changed && <p className="text-sm font-medium text-danger">{t('rbak.hostKey.changedWarning')}</p>}
                    <div className="flex flex-wrap gap-2">
                        <Button type="button" variant={changed ? 'danger' : 'primary'} icon={Check} disabled={busy}
                            className="max-sm:min-h-11" onClick={() => onConfirm(result.hostKey!.fingerprint)}>
                            {t('rbak.hostKey.match')}
                        </Button>
                        <Button type="button" icon={X} disabled={busy} className="max-sm:min-h-11" onClick={onReject}>
                            {t('rbak.hostKey.noMatch')}
                        </Button>
                    </div>
                </div>
            ) : result.guidance && !result.ok && <GuidanceBox guidance={result.guidance} context="test" />}
        </div>
    );
}

function RemoveDialog({ destination, answer, onClose, onRemoved }: {
    destination: RemoteDestination;
    answer: LostAnswerHandle;
    onClose: () => void;
    onRemoved: () => void;
}) {
    const { t } = useI18n();
    const [removing, setRemoving] = useState(false);
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const remove = async () => {
        setRemoving(true);
        setRefusal(null);
        try {
            const res = await answer.send(`${REMOTE_DESTINATIONS_URL}/${destination.id}`, { method: 'DELETE' }, {
                shows: (read) => {
                    const value = read[0]?.value as { destinations?: { id: number }[] } | undefined;
                    return value?.destinations ? !value.destinations.some((d) => d.id === destination.id) : null;
                },
                made: () => onRemoved(),
            });
            if (!res) return;
            if (!res.ok) {
                setRefusal(await readApiError(res));
                return;
            }
            answer.settle();
            showToast('success', t('rbak.remove.done'));
            onRemoved();
        } finally {
            setRemoving(false);
        }
    };
    return (
        <Dialog
            id="rbak-remove"
            title={t('rbak.remove.title', { name: destination.name })}
            description={t('rbak.remove.body')}
            icon={Trash2}
            tone="danger"
            busy={removing}
            onDismiss={onClose}
            actions={
                <>
                    <Button type="button" onClick={onClose} disabled={removing}>{t('rbak.cancel')}</Button>
                    <Button type="button" variant="danger" icon={Trash2} loading={removing} disabled={answer.holding} onClick={() => void remove()}>
                        {t('rbak.remove.confirm')}
                    </Button>
                </>
            }
        >
            {(refusal || answer.lost) ? (
                <div className="space-y-3">
                    <ResultUnknown answer={answer} />
                    <RemoteRefusal problem={refusal} sentence="rbak.remove.failed" />
                </div>
            ) : null}
        </Dialog>
    );
}

// ----- the administrator's every-site schedules -----

function ServerPolicies({ destinations }: { destinations: RemoteDestination[] }) {
    const { t } = useI18n();
    const policies = useRemote(REMOTE_POLICIES_URL, decodePolicies);
    const answer = useLostAnswer(() => policies.retry());
    const [editing, setEditing] = useState<{ policy: RemotePolicy | null } | null>(null);
    const [working, setWorking] = useState<number | 'new' | null>(null);
    const [refusal, setRefusal] = useState<{ problem: ApiError; sentence: TranslationKey } | null>(null);
    const known = policies.remote.state === 'known' && !policies.reading && !answer.holding;

    const save = async (body: ScheduleBody) => {
        setWorking(editing?.policy?.destinationId ?? 'new');
        setRefusal(null);
        try {
            const res = await answer.send(REMOTE_POLICIES_URL, { method: 'PUT', headers: JSON_HEADERS, body: JSON.stringify(body) });
            if (!res) return;
            if (!res.ok) {
                setRefusal({ problem: await readApiError(res), sentence: 'rbak.sched.saveFailed' });
                return;
            }
            answer.settle();
            showToast('success', t('rbak.sched.saved'));
            setEditing(null);
            void policies.retry();
            invalidate('remoteBackups');
        } finally {
            setWorking(null);
        }
    };

    const turnOff = async (policy: RemotePolicy) => {
        setWorking(policy.destinationId);
        setRefusal(null);
        try {
            const res = await answer.send(`${REMOTE_POLICIES_URL}?destination_id=${policy.destinationId}`, { method: 'DELETE' });
            if (!res) return;
            if (!res.ok) {
                setRefusal({ problem: await readApiError(res), sentence: 'rbak.sched.offFailed' });
                return;
            }
            answer.settle();
            showToast('success', t('rbak.sched.offDone'));
            void policies.retry();
            invalidate('remoteBackups');
        } finally {
            setWorking(null);
        }
    };

    return (
        <section className="space-y-3 rounded-xl border border-border bg-surface p-5" aria-labelledby="rbak-policies-title" aria-busy={policies.reading}>
            <div>
                <h3 id="rbak-policies-title" className="flex items-center gap-2 text-sm font-semibold text-fg">
                    <CalendarClock className="h-4 w-4 text-primary" aria-hidden="true" />
                    {t('rbak.policies.title')}
                </h3>
                <p className="mt-1 max-w-[75ch] text-sm text-fg-muted">{t('rbak.policies.intro')}</p>
            </div>
            <ResultUnknown answer={answer} />
            {refusal && <RemoteRefusal problem={refusal.problem} sentence={refusal.sentence} />}
            <RemoteGate remote={policies.remote} checking={t('rbak.policies.checking')} failed={t('rbak.policies.unknown')}
                onRetry={() => void policies.retry()} busy={policies.reading}>
                {(shown) => (
                    <div className="space-y-2">
                        {shown.value.length === 0 && <p className="text-sm text-fg-muted">{t('rbak.policies.empty')}</p>}
                        <ul className="space-y-2">
                            {shown.value.map((policy) => (
                                <li key={policy.destinationId} className="rounded-lg border border-border p-3">
                                    {editing?.policy?.destinationId === policy.destinationId ? (
                                        <ScheduleEditor idPrefix={`rbak-policy-${policy.destinationId}`} destinations={destinations} initial={policy}
                                            busy={working !== null} onSave={(body) => void save(body)} onCancel={() => setEditing(null)} />
                                    ) : (
                                        <PolicyLine policy={policy} disabled={!known || shown.stale || working !== null}
                                            busy={working === policy.destinationId}
                                            onEdit={() => setEditing({ policy })} onOff={() => void turnOff(policy)} />
                                    )}
                                </li>
                            ))}
                        </ul>
                        {editing && editing.policy === null ? (
                            <ScheduleEditor idPrefix="rbak-policy-new"
                                destinations={destinations.filter((d) => !shown.value.some((p) => p.destinationId === d.id))}
                                busy={working !== null} onSave={(body) => void save(body)} onCancel={() => setEditing(null)} />
                        ) : destinations.length === 0 ? (
                            <p className="text-sm text-fg-muted">{t('rbak.policies.needServer')}</p>
                        ) : destinations.some((d) => !shown.value.some((p) => p.destinationId === d.id)) && (
                            <Button icon={Plus} disabled={!known || shown.stale || working !== null} className="max-sm:min-h-11"
                                onClick={() => setEditing({ policy: null })}>
                                {t('rbak.policies.add')}
                            </Button>
                        )}
                    </div>
                )}
            </RemoteGate>
        </section>
    );
}
