import { useEffect, useRef, useState } from 'react';
import { ShieldCheck, ShieldOff, DownloadCloud } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { Button, Dialog, KeepWords } from './ui';
import { readApiError, apiErrorText, type ApiError } from '../lib/apiError';
import {
    FirewallNoSSHAcknowledgement,
    FirewallSSHReasonLine,
    readFirewallSSHReason,
} from './FirewallSSHNotice';
import { Link } from '../router';

// The firewall box, in its own module since v0.1.0-alpha.98 (D-037): Server →
// Security draws it open with its settings, and Components keeps its one calm
// line. It was part of ServiceList.tsx until then; the Security page should not
// have to load the whole Components page to show it.
// Güvenlik duvarı kutusu alpha.98'den beri kendi modülündedir: Güvenlik sayfası
// onu ayarlarıyla açık çizer, Bileşenler tek sakin satırını tutar.

// Firewall status + toggle. Default-deny inbound: only the panel port, SSH
// (auto-detected, never closable) and installed services' ports are open. The
// open set tracks the running services, so the box exposes only what it runs.
// Güvenlik duvarı durumu + anahtar. Varsayılan-reddet gelen: yalnız panel
// portu, SSH (otomatik tespit, asla kapatılamaz) ve kurulu servis portları
// açık. Açık küme koşan servisleri izler; kutu yalnız koşturduğunu açar.
//
// Its place is Server → Security (D-037, v0.1.0-alpha.98): there the box is
// always open with its settings. Components keeps the one calm line of a
// healthy firewall, whose Settings link opens /security#firewall; a firewall
// that is off, unreadable, without an engine or needing attention still shows
// its box here too, where the owner is when it matters.
// Yeri Sunucu → Güvenlik'tir; orada kutu ayarlarıyla hep açıktır. Bileşenler
// sağlıklı duvarın tek sakin satırını tutar; Ayarlar bağlantısı
// /security#firewall'u açar. Kapalı, okunamayan, motorsuz ya da ilgi isteyen
// duvar kutusunu burada da gösterir.
export function FirewallBar({
    place,
    readiness,
    readinessMessage,
    onRefreshReadiness,
    refreshKey,
}: {
    place: 'components' | 'security';
    readiness: { ready: boolean } | null;
    readinessMessage: string;
    onRefreshReadiness: () => Promise<boolean>;
    // Changes with each scan the page receives; the state is read again then.
    refreshKey: number;
}) {
    const { t } = useI18n();
    type FirewallViewStatus = {
        enabled: boolean;
        engine_available?: boolean;
        tcp_ports?: number[];
        udp_ports?: number[];
        ssh_ports?: number[];
        persistence_state?: string;
        persistence_error?: string;
        snapshot_version?: number;
        ssh_discovery_reason?: string;
    };
    const [st, setSt] = useState<FirewallViewStatus | null>(null);
    const [statusError, setStatusError] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);
    const [pendingAction, setPendingAction] = useState<'enable' | 'disable' | 'save' | null>(null);
    const [noSSHAcknowledged, setNoSSHAcknowledged] = useState(false);
    const sshReason = readFirewallSSHReason(st?.ssh_discovery_reason);

    // A failed status read is neither "on" nor "off". Preserve that distinction
    // so an agent/permission/network failure can never be mistaken for an open firewall.
    // Başarısız durum okuması ne "açık" ne de "kapalı"dır. Agent/yetki/ağ
    // hatasının açık bir güvenlik duvarıyla karıştırılmaması için bu ayrımı koru.
    // Only the latest read may set what the banner says.
    // Bandin soyledigini yalniz en son okuma belirler.
    const loadSeq = useRef(0);
    const load = async () => {
        const seq = ++loadSeq.current;
        try {
            const r = await fetch('/api/v1/firewall', { cache: 'no-store' });
            let body: unknown;
            try {
                body = await r.json();
            } catch {
                throw new Error(
                    r.ok
                        ? t('firewall.invalidResponse')
                        : `${t('firewall.loadFailed')} (HTTP ${r.status})`,
                );
            }

            // A coded answer is said in the interface language (err.<CODE>, for
            // example err.AGENT_UNREACHABLE); only an uncoded one keeps the
            // server's own words (sixteenth record, O4).
            // Kodlu yanit arayuz dilinde soylenir; yalniz kodsuz olan sunucunun
            // kendi sozlerini korur.
            const record = typeof body === 'object' && body !== null ? body as Record<string, unknown> : null;
            const apiError: ApiError | null =
                record && typeof record.error === 'string' && record.error.trim()
                    ? {
                        message: record.error,
                        code: typeof record.code === 'string' && record.code ? record.code : undefined,
                        reason: typeof record.reason === 'string' && record.reason ? record.reason : undefined,
                    }
                    : null;
            if (!r.ok) {
                throw new Error(apiError ? apiErrorText(apiError, t, 'firewall.loadFailed') : `${t('firewall.loadFailed')} (HTTP ${r.status})`);
            }
            if (apiError) {
                throw new Error(apiErrorText(apiError, t, 'firewall.loadFailed'));
            }
            if (
                typeof body !== 'object' ||
                body === null ||
                !('enabled' in body) ||
                typeof body.enabled !== 'boolean'
            ) {
                throw new Error(t('firewall.invalidResponse'));
            }

            if (seq !== loadSeq.current) return;
            setSt(body as FirewallViewStatus);
            setStatusError(null);
        } catch (e) {
            if (seq !== loadSeq.current) return;
            setSt(null);
            setStatusError(e instanceof Error && e.message ? e.message : t('firewall.loadFailed'));
        }
    };
    // Read on mount and again with every scan the page receives.
    // Acilista ve sayfanin aldigi her taramada yeniden okunur.
    useEffect(() => {
        void load();
    }, [refreshKey]);

    const toggle = async () => {
        if (!st) return;
        const enabling = !st.enabled;
        setBusy(true);
        try {
            const r = await fetch('/api/v1/firewall', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                // The no-SSH acknowledgement is its own field and only ever
                // travels with a request that is turning the firewall on.
                // SSH'sız onayı kendi alanıdır ve yalnız güvenlik duvarını
                // açan bir istekle birlikte yola çıkar.
                body: JSON.stringify(
                    enabling && noSSHAcknowledged
                        ? { enabled: true, no_ssh_acknowledged: true }
                        : { enabled: enabling },
                ),
            });
            if (!r.ok) {
                // A server can change between the status read and the change.
                // Re-read it so the screen offers the same way forward it
                // would have offered had it known.
                // Sunucu, durum okumasıyla değişiklik arasında değişebilir.
                // Ekranın, bilseydi sunacağı yolu yine sunması için durumu
                // yeniden oku.
                void load();
                showToast('error', apiErrorText(await readApiError(r), t, 'firewall.changeFailed'));
                return;
            }
            const d = await r.json();
            if (d.error) throw new Error(t('firewall.changeFailed'));
            setSt(d);
            showToast('success', d.enabled ? t('firewall.onDone') : t('firewall.offDone'));
        } catch (e) {
            showToast('error', e instanceof Error && e.message ? e.message : t('common.error'));
        } finally {
            setBusy(false);
            void onRefreshReadiness();
        }
    };

    const saveForReboot = async () => {
        setBusy(true);
        try {
            const r = await fetch('/api/v1/firewall', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ action: 'save_for_reboot' }),
            });
            if (!r.ok) {
                showToast('error', apiErrorText(await readApiError(r), t, 'firewall.persistenceSaveFailed'));
                return;
            }
            const d = await r.json();
            if (d.error) throw new Error(t('firewall.persistenceSaveFailed'));
            if (d.persistence_state !== 'ready') throw new Error(d.persistence_error || t('firewall.persistenceSaveFailed'));
            setSt(d);
            showToast('success', t('firewall.persistenceSaved'));
        } catch (e) {
            showToast('error', e instanceof Error && e.message ? e.message : t('firewall.persistenceSaveFailed'));
        } finally {
            setBusy(false);
            void onRefreshReadiness();
        }
    };

    const requestAction = (action: 'enable' | 'disable' | 'save') => {
        if (readiness?.ready !== true) {
            showToast('warning', readinessMessage || t('services.mutationReadiness.checking'));
            return;
        }
        setNoSSHAcknowledged(false);
        setPendingAction(action);
    };

    const closeAction = () => {
        setNoSSHAcknowledged(false);
        setPendingAction(null);
    };

    const confirmAction = async () => {
        const action = pendingAction;
        if (!action || readiness?.ready !== true) return;
        if (action === 'enable' && sshReason === 'no_ssh_service' && !noSSHAcknowledged) return;
        setPendingAction(null);
        if (action === 'save') await saveForReboot();
        else await toggle();
    };

    // A read that failed is not a failure of the firewall: it stands on the
    // attention surface with ink text, never in the failure colour (DESIGN.md,
    // "could not check"; browser inspection 2026-10-14). The text keeps a
    // readable width: on a phone the unavailable control goes under it
    // instead of squeezing it to half the card.
    // Basarisiz okuma guvenlik duvarinin basarisizligi degildir: dikkat
    // yuzeyinde, murekkep metinle durur; telefonda denetim metnin altina iner.
    if (statusError) {
        return (
            <section role="alert" className="mb-4 rounded-xl border border-warning-mark/60 bg-warning-mark/20 p-4" data-firewall-unread="">
                <div className="flex flex-wrap items-start gap-3">
                    <ShieldOff className="mt-0.5 h-5 w-5 shrink-0 text-warning" aria-hidden="true" />
                    <div className="min-w-0 flex-1 basis-64">
                        <div className="text-sm font-semibold text-fg">
                            {t('firewall.title')}{' '}
                            <span>{t('firewall.unknown')}</span>
                        </div>
                        <p className="text-xs text-fg-muted">{t('firewall.unknownHint')}</p>
                        <p className="mt-1 break-words text-xs text-fg"><KeepWords text={statusError} /></p>
                    </div>
                    <Button type="button" variant="secondary" disabled>
                        {t('firewall.controlUnavailable')}
                    </Button>
                </div>
            </section>
        );
    }

    if (!st) return null;
    const openTcp = st.tcp_ports || [];
    const openUdp = st.udp_ports || [];
    const persistenceWarning = st.persistence_state === 'stale'
        ? t('firewall.persistence.stale')
        : st.persistence_state === 'invalid'
            ? t('firewall.persistence.invalid')
            : st.persistence_state === 'unverified'
                ? t('firewall.persistence.unverified')
                : '';

    // No engine → the box is exposed (no firewall) AND "Turn on" cannot work.
    // Route the operator to install the engine (it's the nftables card in the
    // Security section on this very page) rather than fail an opaque toggle.
    // Motor yok → kutu açık (duvar yok) VE "Turn on" çalışamaz. Anlamsız bir
    // hata yerine operatörü motoru kurmaya yönlendir (bu sayfadaki Güvenlik
    // bölümündeki nftables kartı).
    if (st.engine_available === false) {
        return (
            <section className="mb-4 rounded-xl border border-warning/50 bg-warning-mark/20 p-4">
                <div className="flex flex-wrap items-center gap-3">
                    <ShieldOff className="h-5 w-5 shrink-0 text-warning" />
                    <div className="min-w-0 flex-1">
                        <div className="text-sm font-semibold text-fg">
                            {t('firewall.title')} <span className="text-warning">{t('firewall.noEngine')}</span>
                        </div>
                        <p className="text-xs text-fg-muted">{t('firewall.noEngineHint')}</p>
                    </div>
                </div>
            </section>
        );
    }

    // Off is the state that must not look calm: every port is exposed, so the
    // whole banner turns amber. On earns a quiet green tint.
    // Kapalı hal sakin GÖRÜNMEMESİ gereken haldir: tüm portlar açıktır, bu
    // yüzden bandın tamamı ambere döner. Açık hal sessiz yeşil tondadır.
    // Active, readable, saved for a reboot and nothing to say about SSH: one
    // calm line (alpha.94, critique item 7). The box stays for a firewall
    // that is off, unreadable, without an engine or needing attention; its
    // settings are on the Security page (alpha.98), where the box is open.
    // Açık, okunabilir, yeniden başlatma için kayıtlı ve SSH hakkında
    // söylenecek bir şey yok: tek sakin satır. Kutu kapalı, okunamayan,
    // motoru olmayan ya da ilgi isteyen duvar için kalır.
    const healthy = st.enabled
        && (st.persistence_state === undefined || st.persistence_state === '' || st.persistence_state === 'ready')
        && !sshReason;
    if (healthy && place === 'components') {
        return (
            <section className="mb-4 flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-fg-muted" data-firewall-compact="">
                <ShieldCheck className="h-4 w-4 shrink-0 text-success" aria-hidden="true" />
                <span className="font-medium text-fg">{t('firewall.compact.on')}</span>
                <span className="text-fg-subtle" aria-hidden="true">·</span>
                <span>{t('firewall.compact.ports', { n: openTcp.length + openUdp.length })}</span>
                <span className="text-fg-subtle" aria-hidden="true">·</span>
                <Link
                    to="/security#firewall"
                    className="font-semibold text-primary underline decoration-1 underline-offset-[3px] hover:decoration-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary max-sm:inline-flex max-sm:min-h-11 max-sm:items-center"
                    data-firewall-settings-link=""
                >
                    {t('firewall.compact.settings')}
                </Link>
            </section>
        );
    }

    return (
        <>
        <section
            className={`mb-4 rounded-xl border p-4 ${
                st.enabled ? 'border-border bg-surface' : 'border-warning/50 bg-warning-mark/20'
            }`}
        >
            <div className="flex flex-wrap items-center gap-3">
                {st.enabled ? <ShieldCheck className="h-5 w-5 text-success" /> : <ShieldOff className="h-5 w-5 text-warning" />}
                <div className="min-w-0 flex-1">
                    <div className="text-sm font-semibold text-fg">
                        {t('firewall.title')}{' '}
                        <span className={st.enabled ? 'text-success' : 'text-warning'}>
                            {st.enabled ? t('firewall.on') : t('firewall.off')}
                        </span>
                    </div>
                    <p className="text-xs text-fg-muted">
                        {st.enabled ? t('firewall.onHint') : t('firewall.offHint')}
                    </p>
                    {sshReason && <FirewallSSHReasonLine reason={sshReason} />}
                    {st.enabled && st.persistence_state === 'missing' && (
                        <p className="mt-1 text-xs text-warning">{t('firewall.persistenceMissing')}</p>
                    )}
                    {st.persistence_state && ['stale', 'invalid', 'unverified'].includes(st.persistence_state) && (
                        <p className="mt-1 text-xs text-danger">
                            {persistenceWarning}
                            {st.persistence_error ? `: ${st.persistence_error}` : ''}
                        </p>
                    )}
                </div>
                {st.enabled && st.persistence_state === 'missing' && (
                    <button
                        onClick={() => requestAction('save')}
                        disabled={busy || readiness?.ready !== true}
                        className="rounded-lg border border-warning/50 bg-warning-mark/20 px-3 py-1.5 text-xs font-semibold text-warning transition-colors hover:bg-warning/20 disabled:opacity-50"
                    >
                        {t('firewall.saveForReboot')}
                    </button>
                )}
                <button
                    onClick={() => requestAction(st.enabled ? 'disable' : 'enable')}
                    disabled={busy || readiness?.ready !== true}
                    className={`rounded-lg px-3 py-1.5 text-xs font-semibold transition-colors disabled:opacity-50 ${
                        st.enabled
                            ? 'border border-border-strong bg-surface text-fg hover:bg-surface-2'
                            : 'bg-primary text-primary-fg hover:bg-primary/90'
                    }`}
                >
                    {st.enabled ? t('firewall.turnOff') : t('firewall.turnOn')}
                </button>
            </div>
            {st.enabled && (openTcp.length > 0 || openUdp.length > 0) && (
                <div className="mt-3 flex flex-wrap gap-1.5">
                    {openTcp.map((p) => (
                        <span key={`t${p}`} className="rounded bg-surface-2 px-2 py-0.5 font-mono text-xs text-fg-muted">
                            {p}/tcp{st.ssh_ports?.includes(p) ? ' (SSH)' : ''}
                        </span>
                    ))}
                    {openUdp.map((p) => (
                        <span key={`u${p}`} className="rounded bg-surface-2 px-2 py-0.5 font-mono text-xs text-fg-muted">{p}/udp</span>
                    ))}
                </div>
            )}
        </section>
        {pendingAction && (
            <FirewallActionConfirmationDialog
                action={pendingAction}
                confirmDisabled={busy || readiness?.ready !== true}
                noSSHService={pendingAction === 'enable' && sshReason === 'no_ssh_service'}
                noSSHAcknowledged={noSSHAcknowledged}
                onAcknowledgeNoSSH={setNoSSHAcknowledged}
                onCancel={closeAction}
                onConfirm={() => void confirmAction()}
            />
        )}
        </>
    );
}

function FirewallActionConfirmationDialog({
    action,
    confirmDisabled,
    noSSHService,
    noSSHAcknowledged,
    onAcknowledgeNoSSH,
    onCancel,
    onConfirm,
}: {
    action: 'enable' | 'disable' | 'save';
    confirmDisabled: boolean;
    noSSHService: boolean;
    noSSHAcknowledged: boolean;
    onAcknowledgeNoSSH: (value: boolean) => void;
    onCancel: () => void;
    onConfirm: () => void;
}) {
    const { t } = useI18n();
    const destructive = action === 'disable';
    const Icon = action === 'enable' ? ShieldCheck : action === 'disable' ? ShieldOff : DownloadCloud;
    return (
        <Dialog
            id='firewall-action-confirm'
            tone={destructive ? 'danger' : 'default'}
            icon={Icon}
            title={t(`firewall.confirm.${action}.title` as Parameters<typeof t>[0])}
            description={t(`firewall.confirm.${action}.description` as Parameters<typeof t>[0])}
            onDismiss={onCancel}
            footerLead={noSSHService ? (
                <FirewallNoSSHAcknowledgement
                    id='firewall-action-no-ssh'
                    checked={noSSHAcknowledged}
                    disabled={confirmDisabled}
                    onChange={onAcknowledgeNoSSH}
                />
            ) : undefined}
            actions={
                <>
                    <Button variant='secondary' autoFocus onClick={onCancel}>
                        {t('common.cancel')}
                    </Button>
                    <Button
                        variant={destructive ? 'danger' : 'primary'}
                        icon={Icon}
                        disabled={confirmDisabled || (noSSHService && !noSSHAcknowledged)}
                        onClick={onConfirm}
                    >
                        {noSSHService
                            ? t('firewall.ssh.no_ssh_service.confirm')
                            : t(`firewall.confirm.${action}.button` as Parameters<typeof t>[0])}
                    </Button>
                </>
            }
        />
    );
}
