import { useState } from 'react';
import { FileKey, ShieldCheck } from 'lucide-react';
import { Link } from '../router';
import { useMaintenanceWords, type MaintenanceWordKey } from '../i18n/serverMaintenance';
import { readApiError, refusalText, type ApiError } from '../lib/apiError';
import { confirmAction, type OperationEnd } from '../lib/confirmAction';
import { useRemote } from '../lib/remote';
import { Button, Dialog, ErrorBanner, Field, inputClass, KeepWords, RemoteGate } from './ui';
import { LifeLine, lifeDate, type Tone } from './LifeLine';

// Server → Maintenance → Certificates (alpha.128, G31): one read-only list
// of every certificate this server uses, what uses it, when it expires and
// whether it renews by itself (cmd/panel/certificate_overview.go), and the
// certificate requests (CSR) for a certificate bought elsewhere. The private
// key of a request is made and kept by the Agent; this screen only ever has
// the request itself, to copy or download. Discarding asks first and runs in
// the progress window. Let's Encrypt renewal has no switch here.
//
// Sunucu sertifikaları: tek salt-okunur liste ve CSR. Özel anahtar sunucuda
// kalır; bu ekran yalnız isteği görür.

export const CERTIFICATES_URL = '/api/v1/maintenance/certificates';
const REQUESTS_URL = '/api/v1/maintenance/certificate-requests';
const KEY_TYPES = ['ecdsa-p256', 'rsa-2048', 'rsa-3072', 'rsa-4096'] as const;
const SOON_DAYS = 14;

interface CertificateItem {
    kind: 'panel' | 'default_https' | 'mail_host' | 'site';
    name: string;
    names?: string[];
    source: string;
    issuer?: string;
    state: string;
    expires_at?: string;
    days_left?: number;
    renewal: string;
    used_by: string[];
    domain_id?: number;
}

export interface CertificateRequestItem {
    id: number;
    common_name: string;
    names: string[];
    key_type: string;
    csr_pem: string;
    public_key_sha256: string;
    key_file: string;
    created_at: string;
}

interface Overview { certificates: CertificateItem[]; requests: CertificateRequestItem[] }

export function decodeCertificatesOverview(raw: unknown): Overview {
    const body = raw as Partial<Overview> | null;
    if (!body || !Array.isArray(body.certificates) || !Array.isArray(body.requests)) throw new Error('shape');
    return { certificates: body.certificates, requests: body.requests };
}

const SOURCES = new Set(['letsencrypt', 'uploaded', 'self_signed', 'external', 'none', 'unknown']);
const STATES = new Set(['not_needed', 'pending', 'failed', 'owner', 'unknown']);
const USES = new Set(['panel', 'default_https', 'mail_host', 'website', 'mail']);

function CertificateRow({ item }: { item: CertificateItem }) {
    const { t, locale } = useMaintenanceWords();
    const title = item.kind === 'site' ? item.name : t(`mcert.kind.${item.kind}` as MaintenanceWordKey);
    const source = t(`mcert.source.${SOURCES.has(item.source) ? item.source : 'unknown'}` as MaintenanceWordKey);
    const uses = item.used_by.filter((use) => USES.has(use)).map((use) => t(`mcert.use.${use}` as MaintenanceWordKey, { name: item.name })).join(', ');
    const days = item.days_left;
    const expired = days !== undefined && days < 0;
    const soon = days !== undefined && days >= 0 && days < SOON_DAYS;
    // Let's Encrypt renews about 30 days before the end: a week left means it did not.
    const late = item.renewal === 'automatic' && days !== undefined && days < 7;
    let line: { tone: Tone; text: string } | null = null;
    if (item.kind === 'default_https' && item.state === 'in_use') {
        line = { tone: 'info', text: t(item.source === 'self_signed' ? 'mcert.default.placeholder' : 'mcert.default.reject') };
    } else if (item.kind === 'mail_host' && item.state === 'pending') {
        line = { tone: 'warn', text: t('mcert.mail.pending', { name: item.name }) };
    } else if (STATES.has(item.state)) {
        line = { tone: item.state === 'failed' ? 'bad' : item.state === 'unknown' ? 'info' : item.state === 'pending' ? 'warn' : 'ok', text: t(`mcert.state.${item.state}` as MaintenanceWordKey) };
    }
    const renewal = expired || late
        ? { tone: 'bad' as Tone, text: t(item.renewal === 'automatic' ? 'mcert.renew.late' : 'mcert.renew.soon') }
        : soon && item.renewal !== 'automatic'
            ? { tone: 'warn' as Tone, text: t('mcert.renew.soon') }
            : item.renewal === 'automatic' || item.renewal === 'manual' || item.renewal === 'off'
                ? { tone: (item.renewal === 'automatic' ? 'ok' : 'info') as Tone, text: t(`mcert.renew.${item.renewal}` as MaintenanceWordKey) }
                : null;
    return (
        <li className="rounded-lg border border-border bg-surface-2/50 p-3 text-sm" data-certificate-kind={item.kind} data-certificate-name={item.name}>
            <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
                <p className="min-w-0 break-all font-medium text-fg">{title}</p>
                <p className="text-fg-muted">{source}{item.issuer ? ` · ${item.issuer}` : ''}</p>
            </div>
            {item.kind !== 'site' && item.names && item.names.length > 0 && item.names[0] !== title && (
                <p className="mt-0.5 break-all font-mono text-xs text-fg-subtle">{item.names.join(', ')}</p>
            )}
            {uses && <p className="mt-0.5 text-fg-muted">{t('mcert.usedBy', { list: uses })}</p>}
            {item.expires_at && days !== undefined && (
                <p className={`mt-0.5 ${expired ? 'text-danger' : soon ? 'text-warning' : 'text-fg-muted'}`}>
                    {expired ? t('mcert.expired', { date: lifeDate(item.expires_at, locale) }) : t('mcert.expires', { date: lifeDate(item.expires_at, locale), days })}
                </p>
            )}
            <div className="mt-1 space-y-1">
                {line && <LifeLine tone={line.tone} attr={`certificate-${item.state}`}>{line.text}</LifeLine>}
                {renewal && <LifeLine tone={renewal.tone} attr={`renewal-${item.renewal}`}>{renewal.text}</LifeLine>}
            </div>
            {item.kind === 'site' && (renewal?.tone === 'bad' || renewal?.tone === 'warn' || item.renewal === 'off') && (
                <Link to={`/domains/${encodeURIComponent(item.name)}?tab=ssl`} className="mt-2 inline-block text-sm font-medium text-primary hover:underline">{t('mcert.openSite')}</Link>
            )}
        </li>
    );
}

function downloadCSR(name: string, pemText: string) {
    const url = URL.createObjectURL(new Blob([pemText], { type: 'application/pkcs10' }));
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `${name.replace(/^\*\./, 'wildcard.').replace(/[^a-z0-9.-]/gi, '_')}.csr`;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function CSRBlock({ request }: { request: CertificateRequestItem }) {
    const { t } = useMaintenanceWords();
    const [copy, setCopy] = useState<'idle' | 'copied' | 'failed'>('idle');
    const copyCSR = async () => {
        try {
            await navigator.clipboard.writeText(request.csr_pem);
            setCopy('copied');
        } catch {
            // Clipboard refused (no permission or no secure context): say so.
            setCopy('failed');
        }
    };
    return (
        <div className="mt-2 space-y-2">
            <textarea readOnly value={request.csr_pem} rows={8} className={`${inputClass} font-mono text-xs`} aria-label={t('mcert.req.title')} onFocus={(event) => event.currentTarget.select()} />
            <div className="flex flex-wrap items-center gap-2">
                <Button onClick={() => void copyCSR()} className="max-sm:min-h-11">{t('mcert.req.copy')}</Button>
                <Button onClick={() => downloadCSR(request.common_name, request.csr_pem)} className="max-sm:min-h-11">{t('mcert.req.download')}</Button>
                {copy !== 'idle' && <span className="text-sm text-fg-muted" role="status">{t(copy === 'copied' ? 'mcert.req.copied' : 'mcert.req.copyFailed')}</span>}
            </div>
            <p className="break-all font-mono text-xs text-fg-subtle">{t('mcert.req.fingerprint', { value: request.public_key_sha256 })}</p>
            <p className="text-sm text-fg-muted"><KeepWords text={t('mcert.req.keyKept', { path: request.key_file })} /></p>
            <p className="text-sm text-fg-muted">{t('mcert.req.installLater')}</p>
        </div>
    );
}

function RequestRow({ request, reload }: { request: CertificateRequestItem; reload: () => void }) {
    const { t, locale } = useMaintenanceWords();
    const [open, setOpen] = useState(false);
    const discard = () => void confirmAction({
        title: t('mcert.req.discardTitle', { name: request.common_name }),
        message: t('mcert.req.discardMessage'),
        confirmLabel: t('mcert.req.discardConfirm'),
        tone: 'danger',
        progressTitle: t('mcert.req.discarding'),
        action: async (): Promise<OperationEnd> => {
            let res: Response;
            try {
                res = await fetch(`${REQUESTS_URL}/${request.id}/discard`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
            } catch {
                reload();
                return { state: 'unknown', reason: t('mcert.req.lost') };
            }
            reload();
            if (res.ok) return { state: 'done', message: t('mcert.req.discarded') };
            const problem = await readApiError(res);
            return { state: 'failed', reason: refusalText(problem, t, 'mcert.req.discardFailed' as never) };
        },
    });
    return (
        <li className="rounded-lg border border-border bg-surface-2/50 p-3 text-sm" data-certificate-request={request.id}>
            <div className="flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0">
                    <p className="break-all font-medium text-fg">{request.names.join(', ')}</p>
                    <p className="text-fg-muted">{t('mcert.req.created', { date: lifeDate(request.created_at, locale, true), key: t(`mcert.req.key.${request.key_type}` as MaintenanceWordKey) })}</p>
                </div>
                <div className="flex gap-2">
                    <Button onClick={() => setOpen(!open)} className="max-sm:min-h-11">{t(open ? 'mcert.req.hide' : 'mcert.req.show')}</Button>
                    <Button variant="danger" onClick={discard} className="max-sm:min-h-11">{t('mcert.req.discard')}</Button>
                </div>
            </div>
            {open && <CSRBlock request={request} />}
        </li>
    );
}

function NewRequestDialog({ onClose, reload }: { onClose: () => void; reload: () => void }) {
    const { t } = useMaintenanceWords();
    const [names, setNames] = useState('');
    const [keyType, setKeyType] = useState<(typeof KEY_TYPES)[number]>('ecdsa-p256');
    const [busy, setBusy] = useState(false);
    const [failure, setFailure] = useState<ApiError | null>(null);
    const [made, setMade] = useState<CertificateRequestItem | null>(null);
    const submit = async () => {
        if (busy) return;
        setBusy(true);
        setFailure(null);
        try {
            let res: Response;
            try {
                res = await fetch(REQUESTS_URL, {
                    method: 'POST', headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ names: names.split(/[\s,]+/).filter(Boolean), key_type: keyType }),
                });
            } catch {
                setFailure({ message: t('mcert.req.lost') });
                return;
            }
            if (!res.ok) {
                setFailure(await readApiError(res));
                return;
            }
            setMade((await res.json()) as CertificateRequestItem);
            reload();
        } finally {
            setBusy(false);
        }
    };
    return (
        <Dialog id="certificate-request" title={t('mcert.req.dialogTitle')} busy={busy} onDismiss={onClose} width="lg"
            onSubmit={made ? undefined : (event) => { event.preventDefault(); void submit(); }}
            actions={made
                ? <Button variant="primary" onClick={onClose}>{t('mcert.req.close')}</Button>
                : <>
                    <Button onClick={onClose} disabled={busy}>{t('common.cancel')}</Button>
                    <Button variant="primary" type="submit" loading={busy} disabled={busy || names.trim() === ''}>{t('mcert.req.make')}</Button>
                </>}>
            {made ? (
                <div>
                    <LifeLine tone="ok" attr="request-made">{t('mcert.req.made')}</LifeLine>
                    <CSRBlock request={made} />
                </div>
            ) : (
                <div className="space-y-4">
                    <Field label={t('mcert.req.names')} hint={t('mcert.req.namesHint')} htmlFor="certificate-request-names">
                        <textarea id="certificate-request-names" rows={4} value={names} onChange={(event) => setNames(event.target.value)} className={`${inputClass} font-mono`} spellCheck={false} autoCapitalize="none" disabled={busy} />
                    </Field>
                    <Field label={t('mcert.req.keyType')} htmlFor="certificate-request-key">
                        <select id="certificate-request-key" value={keyType} onChange={(event) => setKeyType(event.target.value as (typeof KEY_TYPES)[number])} className={inputClass} disabled={busy}>
                            {KEY_TYPES.map((type) => <option key={type} value={type}>{t(`mcert.req.key.${type}` as MaintenanceWordKey)}</option>)}
                        </select>
                    </Field>
                    <ErrorBanner error={failure} sentence={'mcert.req.failed' as never} />
                </div>
            )}
        </Dialog>
    );
}

export function ServerCertificatesCard() {
    const { t } = useMaintenanceWords();
    const handle = useRemote(CERTIFICATES_URL, decodeCertificatesOverview);
    const [making, setMaking] = useState(false);
    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-maintenance-section="certificates">
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 flex-1 basis-64">
                    <h2 className="flex items-center gap-2 text-base font-semibold text-fg">
                        <ShieldCheck className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                        {t('mcert.title')}
                    </h2>
                    <p className="mt-0.5 text-sm text-fg-muted">{t('mcert.desc')}</p>
                </div>
                {handle.remote.state !== 'loading' && (
                    <Button onClick={() => void handle.retry()} loading={handle.reading} disabled={handle.reading} className="max-sm:min-h-11">{t('mcert.again')}</Button>
                )}
            </div>
            <div className="mt-4">
                <RemoteGate remote={handle.remote} checking={t('mcert.checking')} failed={t('mcert.failed')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value }) => (
                        <div className="space-y-5">
                            <ul className="space-y-2">
                                {value.certificates.map((item) => <CertificateRow key={`${item.kind}:${item.name}`} item={item} />)}
                            </ul>
                            {!value.certificates.some((item) => item.kind === 'site') && <p className="text-sm text-fg-muted">{t('mcert.noSites')}</p>}
                            <div className="border-t border-border pt-4" data-certificate-requests={value.requests.length}>
                                <div className="flex flex-wrap items-start justify-between gap-3">
                                    <div className="min-w-0 flex-1 basis-64">
                                        <h3 className="flex items-center gap-2 text-sm font-semibold text-fg">
                                            <FileKey className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                                            {t('mcert.req.title')}
                                        </h3>
                                        <p className="mt-0.5 text-sm text-fg-muted">{t('mcert.req.desc')}</p>
                                    </div>
                                    <Button onClick={() => setMaking(true)} className="max-sm:min-h-11" data-certificate-request-new="">{t('mcert.req.new')}</Button>
                                </div>
                                {value.requests.length === 0
                                    ? <p className="mt-3 text-sm text-fg-muted">{t('mcert.req.none')}</p>
                                    : <ul className="mt-3 space-y-2">{value.requests.map((request) => <RequestRow key={request.id} request={request} reload={() => void handle.retry()} />)}</ul>}
                            </div>
                        </div>
                    )}
                </RemoteGate>
            </div>
            {making && <NewRequestDialog onClose={() => setMaking(false)} reload={() => void handle.retry()} />}
        </section>
    );
}
