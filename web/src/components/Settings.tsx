import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import { useSearchParams } from '../router';
import { Lock, BadgeCheck, AlertTriangle, Network, DownloadCloud } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { useAuth } from '../auth/AuthContext';
import { Button, CouldNotCheck, ErrorBanner, RemoteGate, inputClass } from './ui';
import { PageHeader } from './PageHeader';
import { apiErrorText, readApiError, type ApiError } from '../lib/apiError';
import { readRemote, useRemote } from '../lib/remote';
import { ServerSetupSettings } from './ServerSetupChoice';
import { availablePanelUpdate, usePanelUpdateDiscovery, wholeVersion } from '../lib/panelUpdateDiscovery';

const LicensePanel = lazy(() => import('./LicensePanel').then((module) => ({ default: module.LicensePanel })));
const PanelUpdateCard = lazy(() => import('./PanelUpdateCard').then((module) => ({ default: module.PanelUpdateCard })));

// Settings holds what is chosen once and decides how the panel behaves
// (D-037, owner 2026-10-22). The person's own account security is My account
// (personal menu); server security, DNS and peer servers and the system
// databases are pages under Server. Their old addresses still arrive there
// (nav.ts, settingsRedirect). No administrator-wide security policy exists
// yet, so there is no section for one.
// Ayarlar, bir kez seçilip panelin davranışını belirleyenleri tutar (D-037).
// Kişinin hesap güvenliği Hesabım'da; sunucu güvenliği, DNS ve eş sunucular ve
// sistem veritabanları Sunucu altındaki sayfalardır. Eski adresleri oraya
// ulaşır. Tüm yöneticiler için bir güvenlik kuralı henüz yok; bölümü de yok.
type SettingsSectionID = 'panel' | 'license' | 'updates' | 'setup';
type SettingsSection = {
    id: SettingsSectionID;
    icon: React.ComponentType<{ className?: string }>;
    title: string;
    description: string;
    /** A quiet note in place of the description, with a dot (D-035). */
    note?: string;
    /** The version inside the note, kept whole on screen. */
    noteVersion?: string;
};

// Each operational concern has its own URL-addressable section so navigation
// and the visible workspace always describe the same task.
// Her operasyonel alan URL ile adreslenebilen ayrı bir bölümdür; böylece
// gezinme ile görünen çalışma alanı her zaman aynı işi anlatır.
export function Settings() {
    const { t } = useI18n();
    const { role } = useAuth();
    const [searchParams, setSearchParams] = useSearchParams();
    // The Panel's stored update check: the updates item says "Update
    // available" when a newer signed release was found (D-035). A read only.
    const updateTarget = availablePanelUpdate(usePanelUpdateDiscovery(role === 'admin').value);
    // Only an administrator reaches this page; /settings takes everyone else
    // to My account before it is drawn (App.tsx, SettingsRoute).
    // Bu sayfaya yalnız yönetici ulaşır; diğerlerini /settings Hesabım'a götürür.
    const sections: SettingsSection[] = role === 'admin'
        ? [
            {
                id: 'panel' as const,
                icon: Lock,
                title: t('settings.section.panel'),
                description: t('settings.section.panel.desc'),
            },
            {
                id: 'license' as const,
                icon: BadgeCheck,
                title: t('settings.section.license'),
                description: t('settings.section.license.desc'),
            },
            {
                id: 'updates' as const,
                icon: DownloadCloud,
                title: t('settings.section.updates'),
                description: t('settings.section.updates.desc'),
                ...(updateTarget ? { note: t('settings.section.updates.available', { version: updateTarget.version }), noteVersion: updateTarget.version } : {}),
            },
            { id: 'setup' as const, icon: Network, title: t('setup.settingsTitle'), description: t('setup.settingsDescription') },
        ]
        : [];
    const requestedSection = searchParams.get('section');
    const activeSection = sections.find((section) => section.id === requestedSection) ?? sections[0];

    useEffect(() => {
        if (!activeSection || requestedSection === activeSection.id) return;
        const next = new URLSearchParams(searchParams);
        next.set('section', activeSection.id);
        setSearchParams(next, { replace: true });
    }, [activeSection?.id, requestedSection, searchParams, setSearchParams]);

    const selectSection = (section: SettingsSectionID) => {
        if (!activeSection || section === activeSection.id) return;
        const next = new URLSearchParams(searchParams);
        next.set('section', section);
        setSearchParams(next);
    };

    const moveSection = (event: React.KeyboardEvent<HTMLButtonElement>, currentIndex: number) => {
        const keyOffsets: Record<string, number> = {
            ArrowRight: 1,
            ArrowDown: 1,
            ArrowLeft: -1,
            ArrowUp: -1,
        };
        let nextIndex = currentIndex;
        if (event.key === 'Home') nextIndex = 0;
        else if (event.key === 'End') nextIndex = sections.length - 1;
        else if (event.key in keyOffsets) {
            nextIndex = (currentIndex + keyOffsets[event.key] + sections.length) % sections.length;
        } else {
            return;
        }
        event.preventDefault();
        const nextSection = sections[nextIndex];
        selectSection(nextSection.id);
        window.requestAnimationFrame(() => {
            document.getElementById(`settings-${nextSection.id}-tab`)?.focus();
        });
    };

    return (
        <div className="p-4 sm:p-6 md:p-8">
            <PageHeader title={t('nav.settings')} subtitle={t('settings.subtitle')} breadcrumb={[t('common.home'), t('nav.settings')]} />
            {activeSection && <SettingsWorkspace
                sections={sections}
                activeID={activeSection.id}
                role={role}
                label={t('settings.sections')}
                onSelect={selectSection}
                onKeyDown={moveSection}
            />}
        </div>
    );
}

function SettingsWorkspace({
    sections,
    activeID,
    role,
    label,
    onSelect,
    onKeyDown,
}: {
    sections: SettingsSection[];
    activeID: SettingsSectionID;
    role: string;
    label: string;
    onSelect: (section: SettingsSectionID) => void;
    onKeyDown: (event: React.KeyboardEvent<HTMLButtonElement>, index: number) => void;
}) {
    const { t } = useI18n();
    return (
        <div className="grid max-w-7xl gap-5 lg:grid-cols-[17rem_minmax(0,1fr)] lg:items-start">
            <SettingsSectionTabs
                sections={sections}
                activeID={activeID}
                label={label}
                onSelect={onSelect}
                onKeyDown={onKeyDown}
            />
            <div className="min-w-0">
                {role === 'admin' && (
                    <>
                        <div id="settings-panel-panel" role="tabpanel" aria-labelledby="settings-panel-tab" hidden={activeID !== 'panel'}>
                            <PanelCertificatePanel active={activeID === 'panel'} />
                        </div>
                        <div id="settings-license-panel" role="tabpanel" aria-labelledby="settings-license-tab" hidden={activeID !== 'license'}>
                            {activeID === 'license' && <Suspense fallback={<p role="status">{t('common.loading')}</p>}><LicensePanel /></Suspense>}
                        </div>
                        <div id="settings-updates-panel" role="tabpanel" aria-labelledby="settings-updates-tab" hidden={activeID !== 'updates'}>
                            {activeID === 'updates' && <Suspense fallback={null}><PanelUpdateCard /></Suspense>}
                        </div>
                        <div id="settings-setup-panel" role="tabpanel" aria-labelledby="settings-setup-tab" hidden={activeID !== 'setup'}>
                            {activeID === 'setup' && <ServerSetupSettings />}
                        </div>
                    </>
                )}
            </div>
        </div>
    );
}

function SettingsSectionTabs({
    sections,
    activeID,
    label,
    onSelect,
    onKeyDown,
}: {
    sections: SettingsSection[];
    activeID: SettingsSectionID;
    label: string;
    onSelect: (section: SettingsSectionID) => void;
    onKeyDown: (event: React.KeyboardEvent<HTMLButtonElement>, index: number) => void;
}) {
    const tabRefs = useRef<Partial<Record<SettingsSectionID, HTMLButtonElement | null>>>({});

    useEffect(() => {
        const activeTab = tabRefs.current[activeID];
        if (!activeTab || typeof window.matchMedia !== 'function' ||
            !window.matchMedia('(max-width: 1023px)').matches) return undefined;
        const frame = window.requestAnimationFrame(() => {
            activeTab.scrollIntoView({ block: 'nearest', inline: 'nearest' });
        });
        return () => window.cancelAnimationFrame(frame);
    }, [activeID]);

    return (
        <nav
            aria-label={label}
            className="flex gap-2 overflow-x-auto rounded-xl border border-border bg-surface p-2 lg:sticky lg:top-6 lg:flex-col lg:overflow-visible"
            role="tablist"
        >
            {sections.map((section, index) => {
                const Icon = section.icon;
                const active = section.id === activeID;
                // relative: the screen-reader-only note is absolutely placed;
                // without a positioned tab it was laid out against the page and
                // widened a phone's Settings to about 730 px (batch 21, alpha.96).
                // relative: yalnız ekran okuyucu notu sekmeye göre yerleşir.
                return (
                    <button
                        key={section.id}
                        ref={(element) => {
                            tabRefs.current[section.id] = element;
                        }}
                        id={`settings-${section.id}-tab`}
                        type="button"
                        role="tab"
                        aria-controls={`settings-${section.id}-panel`}
                        aria-selected={active}
                        tabIndex={active ? 0 : -1}
                        onClick={() => onSelect(section.id)}
                        onKeyDown={(event) => onKeyDown(event, index)}
                        className={`group relative flex min-h-11 shrink-0 items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 lg:w-full ${active
                            ? 'bg-primary/10 text-primary'
                            : 'text-fg-muted hover:bg-surface-subtle hover:text-fg'
                            }`}
                    >
                        <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${active ? 'bg-primary text-primary-fg' : 'bg-surface-subtle text-fg-muted group-hover:text-fg'}`}>
                            <Icon className="h-4 w-4" />
                        </span>
                        <span className="min-w-0">
                            <span className="flex items-center gap-1.5 whitespace-nowrap text-sm font-semibold lg:whitespace-normal">
                                {section.title}
                                {/* The note's dot is primary ink, not a signal
                                    colour: a found update is not a warning. */}
                                {section.note && <span className="h-2 w-2 shrink-0 rounded-full bg-primary" data-settings-note-dot="" aria-hidden="true" />}
                                {section.note && <span className="sr-only lg:hidden">{section.note}</span>}
                            </span>
                            <span className="mt-0.5 hidden text-xs font-normal leading-5 text-fg-muted lg:block" data-settings-note={section.note ? '' : undefined}>
                                {section.note ? wholeVersion(section.note, section.noteVersion ?? '') : section.description}
                            </span>
                        </span>
                    </button>
                );
            })}
        </nav>
    );
}

// The certificate the panel itself serves on its HTTPS port. Out of the box
// it is self-signed (every browser warns); one click issues a Let's Encrypt
// certificate for the panel's domain and restarts the panel to serve it.
// Renewal is automatic afterwards (certbot timer + deploy hook).
// Panelin HTTPS portunda bizzat sunduğu sertifika. Kutudan çıkanı kendinden
// imzalıdır (her tarayıcı uyarır); tek tık, panelin alan adı için Let's
// Encrypt sertifikası alır ve sunması için paneli yeniden başlatır. Sonrası
// otomatik yenilenir (certbot zamanlayıcısı + deploy kancası).
const PANEL_CERTIFICATE_OPERATION_KEY = 'celikpanel.panel-certificate-operation.v1';
const PANEL_CERTIFICATE_POLL_MS = 1500;
const PANEL_CERTIFICATE_MISSING_GRACE_MS = 10 * 60 * 1000;

type PanelCertificateOperationMarker = {
    version: 1;
    request_id: string;
    domain: string;
    created_at: number;
};

type PanelCertificateOperation = {
    id: string;
    request_id: string;
    kind: 'panel_certificate_issue';
    service_id: string;
    status: 'queued' | 'running' | 'succeeded' | 'failed';
    error?: { code: string; message: string };
};

function createPanelCertificateRequestID(): string | null {
    try {
        const bytes = new Uint8Array(16);
        crypto.getRandomValues(bytes);
        return Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('');
    } catch {
        return null;
    }
}

function canonicalPanelCertificateMarkerDomain(raw: string): string {
    const trimmed = raw.trim().toLowerCase();
    return trimmed.endsWith('.') ? trimmed.slice(0, -1) : trimmed;
}

function decodePanelCertificateMarker(raw: string | null): PanelCertificateOperationMarker | null {
    if (!raw || raw.length > 1024) return null;
    try {
        const value = JSON.parse(raw) as Record<string, unknown>;
        if (
            value.version !== 1
            || typeof value.request_id !== 'string'
            || !/^[a-f0-9]{32}$/.test(value.request_id)
            || typeof value.domain !== 'string'
            || value.domain !== canonicalPanelCertificateMarkerDomain(value.domain)
            || !value.domain
            || value.domain.length > 253
            || typeof value.created_at !== 'number'
            || !Number.isFinite(value.created_at)
            || value.created_at <= 0
        ) return null;
        return {
            version: 1,
            request_id: value.request_id,
            domain: value.domain,
            created_at: value.created_at,
        };
    } catch {
        return null;
    }
}

function readPanelCertificateMarker(): PanelCertificateOperationMarker | null {
    try {
        const marker = decodePanelCertificateMarker(
            localStorage.getItem(PANEL_CERTIFICATE_OPERATION_KEY),
        );
        if (marker === null) localStorage.removeItem(PANEL_CERTIFICATE_OPERATION_KEY);
        return marker;
    } catch {
        return null;
    }
}

function storePanelCertificateMarker(marker: PanelCertificateOperationMarker): boolean {
    try {
        localStorage.setItem(PANEL_CERTIFICATE_OPERATION_KEY, JSON.stringify(marker));
        return true;
    } catch {
        return false;
    }
}

function clearPanelCertificateMarker() {
    try {
        localStorage.removeItem(PANEL_CERTIFICATE_OPERATION_KEY);
    } catch {
        // The in-memory marker still gives this tab an authoritative poll key.
    }
}

function decodePanelCertificateOperation(
    payload: unknown,
    marker: PanelCertificateOperationMarker,
): PanelCertificateOperation | null {
    if (!payload || typeof payload !== 'object') return null;
    const operation = (payload as { operation?: unknown }).operation;
    if (!operation || typeof operation !== 'object') return null;
    const value = operation as Record<string, unknown>;
    if (
        typeof value.id !== 'string'
        || !/^[a-f0-9]{32}$/.test(value.id)
        || value.request_id !== marker.request_id
        || value.kind !== 'panel_certificate_issue'
        || value.service_id !== marker.domain
        || (
            value.status !== 'queued'
            && value.status !== 'running'
            && value.status !== 'succeeded'
            && value.status !== 'failed'
        )
    ) return null;
    let operationError: PanelCertificateOperation['error'];
    if (value.error !== undefined) {
        if (!value.error || typeof value.error !== 'object') return null;
        const errorValue = value.error as Record<string, unknown>;
        if (typeof errorValue.code !== 'string' || typeof errorValue.message !== 'string') return null;
        operationError = { code: errorValue.code, message: errorValue.message };
    }
    return {
        id: value.id,
        request_id: marker.request_id,
        kind: 'panel_certificate_issue',
        service_id: marker.domain,
        status: value.status,
        ...(operationError ? { error: operationError } : {}),
    };
}

// What the Panel says about the certificate it serves. `https_enabled: false`
// is the server's answer when it found no certificate it could read; that is
// not "a trusted certificate", which is what an empty issuer used to be drawn
// as.
// Panelin sunduğu sertifika hakkında söylediği. `https_enabled: false`,
// sunucunun okuyabildiği bir sertifika bulamadığındaki yanıtıdır.
interface PanelCertificateInfo {
    https_enabled: boolean;
    self_signed: boolean;
    issuer: string;
    expires_at: string;
}

const PANEL_CERTIFICATE_URL = '/api/v1/panel/certificate';

function decodePanelCertificate(raw: unknown): PanelCertificateInfo {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const body = raw as Record<string, unknown>;
    if (typeof body.https_enabled !== 'boolean' || typeof body.self_signed !== 'boolean') throw new Error('field');
    return {
        https_enabled: body.https_enabled,
        self_signed: body.self_signed,
        issuer: typeof body.issuer === 'string' ? body.issuer : '',
        expires_at: typeof body.expires_at === 'string' ? body.expires_at : '',
    };
}

// The operation endpoint's answer is checked against the exact request by
// decodePanelCertificateOperation; here it only has to be JSON.
const passThrough = (raw: unknown): unknown => raw;

// What became of the last certificate request, as far as the server has said.
// `failed` and `notRecorded` are the server's own answers. A poll that got no
// answer is neither: it is `unconfirmed` below, and it never becomes "failed".
// Son sertifika isteğinin, sunucunun söylediği kadarıyla sonucu. Yanıt
// alınamayan sorgu "başarısız" olmaz; aşağıdaki `unconfirmed` durumudur.
type IssueOutcome =
    | { state: 'none' }
    | { state: 'failed'; domain: string; reason: string }
    | { state: 'notRecorded'; domain: string }
    | { state: 'issued'; domain: string; here: boolean };

// How long polls may go unanswered before the screen says so, and how often it
// asks while it is saying so.
const PANEL_CERTIFICATE_UNCONFIRMED_AFTER_MS = 9000;
const PANEL_CERTIFICATE_UNCONFIRMED_POLL_MS = 5000;
// Long enough to read why the page is about to move, and for the Panel to be
// listening again after its restart.
const PANEL_CERTIFICATE_REOPEN_SECONDS = 10;

function PanelCertificatePanel({ active }: { active: boolean }) {
    const { t, locale } = useI18n();
    const certificate = useRemote(PANEL_CERTIFICATE_URL, decodePanelCertificate);
    const [domain, setDomain] = useState(() =>
        /^[0-9.]+$/.test(window.location.hostname) ? '' : window.location.hostname,
    );
    const [busy, setBusy] = useState(false);
    const [outcome, setOutcome] = useState<IssueOutcome>({ state: 'none' });
    // A refusal that names the place that fixes it (since v0.1.0-alpha.99 the
    // missing contact email, fixed on My account) stays on the card with its
    // button instead of passing as a toast.
    // Çözüldüğü yeri adlandıran ret, düğmesiyle kartta kalır.
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const [unconfirmed, setUnconfirmed] = useState(false);
    const [checkNow, setCheckNow] = useState(0);
    const [stay, setStay] = useState(false);
    const [secondsLeft, setSecondsLeft] = useState(PANEL_CERTIFICATE_REOPEN_SECONDS);
    const [pendingOperation, setPendingOperation] = useState<PanelCertificateOperationMarker | null>(
        () => readPanelCertificateMarker(),
    );
    const issueInFlightRef = useRef(pendingOperation !== null);
    // Only the page that sent the request may move itself to the new address.
    // A marker found in storage belongs to another tab, or to an earlier visit.
    // Yalnız isteği gönderen sayfa kendini yeni adrese taşıyabilir. Depoda
    // bulunan işaret başka bir sekmeye ya da önceki bir ziyarete aittir.
    const startedHereRef = useRef(false);
    const unansweredSinceRef = useRef<number | null>(null);
    const restarting = outcome.state === 'issued';
    const retryCertificate = certificate.retry;

    useEffect(() => {
        const marker = pendingOperation;
        if (marker === null || restarting) return undefined;
        const exactMarker: PanelCertificateOperationMarker = marker;
        let cancelled = false;
        let timer: ReturnType<typeof setTimeout> | undefined;
        setBusy(true);

        function schedule(delay = PANEL_CERTIFICATE_POLL_MS) {
            if (!cancelled) timer = setTimeout(() => void poll(), delay);
        }
        // No answer about this exact request: keep the marker, keep asking,
        // and after a while say that the result is not known.
        // Bu isteğe dair yanıt yok: işaret korunur, sorulmaya devam edilir ve
        // bir süre sonra sonucun bilinmediği söylenir.
        function unanswered() {
            const since = unansweredSinceRef.current ?? Date.now();
            unansweredSinceRef.current = since;
            const late = Date.now() - since >= PANEL_CERTIFICATE_UNCONFIRMED_AFTER_MS;
            if (late) setUnconfirmed(true);
            schedule(late ? PANEL_CERTIFICATE_UNCONFIRMED_POLL_MS : PANEL_CERTIFICATE_POLL_MS);
        }
        function release(next: IssueOutcome) {
            clearPanelCertificateMarker();
            issueInFlightRef.current = false;
            unansweredSinceRef.current = null;
            setUnconfirmed(false);
            setPendingOperation(null);
            setOutcome(next);
        }
        async function poll() {
            const answer = await readRemote(
                `/api/v1/service/operation?request_id=${encodeURIComponent(exactMarker.request_id)}`,
                passThrough,
                undefined,
                { cache: 'no-store' },
            );
            if (cancelled) return;
            if (answer.state !== 'known') {
                // 404 is the server saying it holds no such request. Within
                // the grace period the POST may still be on its way; after it
                // the request was never recorded, which is not a failure of
                // the certificate.
                if (answer.status === 404 && Date.now() - exactMarker.created_at > PANEL_CERTIFICATE_MISSING_GRACE_MS) {
                    release({ state: 'notRecorded', domain: exactMarker.domain });
                    setBusy(false);
                    void retryCertificate();
                    return;
                }
                unanswered();
                return;
            }
            const operation = decodePanelCertificateOperation(answer.value, exactMarker);
            if (operation === null) {
                // A mismatched privileged operation can never authorize
                // clearing or replacing this exact request-id marker.
                unanswered();
                return;
            }
            unansweredSinceRef.current = null;
            setUnconfirmed(false);
            if (operation.status === 'failed') {
                release({ state: 'failed', domain: exactMarker.domain, reason: operation.error?.message ?? '' });
                setBusy(false);
                showToast('error', operation.error?.message || t('panelCert.failed'));
                void retryCertificate();
                return;
            }
            if (operation.status !== 'succeeded') {
                schedule();
                return;
            }
            release({ state: 'issued', domain: exactMarker.domain, here: startedHereRef.current });
            setBusy(false);
            showToast('success', t('panelCert.issued'));
        }
        void poll();
        return () => {
            cancelled = true;
            if (timer !== undefined) clearTimeout(timer);
        };
    }, [pendingOperation, restarting, checkNow, retryCertificate, t]);

    const secureAddress = outcome.state === 'issued'
        ? `https://${outcome.domain}:${window.location.port || '2083'}/`
        : pendingOperation
            ? `https://${pendingOperation.domain}:${window.location.port || '2083'}/`
            : '';
    // The page moves by itself only where the person is looking at the reason:
    // this section, of the tab that asked, and not after "Stay here".
    // Sayfa kendiliğinden yalnız kişinin nedeni gördüğü yerde taşınır: isteği
    // gönderen sekmenin bu bölümünde ve "Burada kal" denmemişse.
    const reopening = outcome.state === 'issued' && outcome.here && active && !stay;

    useEffect(() => {
        // Leaving the section is an answer too: the page is not moved later
        // from under another section.
        if (outcome.state === 'issued' && !active) setStay(true);
    }, [outcome.state, active]);

    useEffect(() => {
        if (!reopening) return undefined;
        if (secondsLeft <= 0) {
            if (document.visibilityState === 'hidden') setStay(true);
            else window.location.href = secureAddress;
            return undefined;
        }
        const timer = setTimeout(() => setSecondsLeft((left) => left - 1), 1000);
        return () => clearTimeout(timer);
    }, [reopening, secondsLeft, secureAddress]);

    const certificateKnown = certificate.remote.state === 'known';

    // What became of a request appears where the person may not be looking
    // (the form is at the end of the card, the result at its start), so it is
    // brought into view when it appears, on the section that is open.
    // İsteğin sonucu kişinin bakmadığı bir yerde belirebilir; açık olan
    // bölümde, belirdiğinde görünür alana getirilir.
    const noticeRef = useRef<HTMLDivElement>(null);
    const noticeShown = outcome.state !== 'none' || (pendingOperation !== null && unconfirmed);
    useEffect(() => {
        if (noticeShown && active) noticeRef.current?.scrollIntoView?.({ block: 'nearest' });
    }, [noticeShown, active, outcome.state]);

    const issue = async () => {
        // The ref closes the pre-render double-click window. A marker loaded
        // during the initial render is authoritative and must never be
        // overwritten by a new request id while exact polling is in flight.
        if (issueInFlightRef.current || pendingOperation !== null || restarting || !domain) return;
        // Nothing is requested about a certificate whose state is not known.
        if (!certificateKnown) return;
        issueInFlightRef.current = true;
        const requestID = createPanelCertificateRequestID();
        const marker: PanelCertificateOperationMarker | null = requestID === null
            ? null
            : {
                version: 1,
                request_id: requestID,
                domain: canonicalPanelCertificateMarkerDomain(domain),
                created_at: Date.now(),
            };
        if (marker === null || !marker.domain || !storePanelCertificateMarker(marker)) {
            issueInFlightRef.current = false;
            showToast('error', t('panelCert.failed'));
            return;
        }
        startedHereRef.current = true;
        unansweredSinceRef.current = null;
        setOutcome({ state: 'none' });
        setRefusal(null);
        setUnconfirmed(false);
        setStay(false);
        setSecondsLeft(PANEL_CERTIFICATE_REOPEN_SECONDS);
        setBusy(true);
        setPendingOperation(marker);
        try {
            const res = await fetch('/api/v1/panel/certificate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ domain, request_id: marker.request_id }),
            });
            if (!res.ok) {
                // 408/429/5xx and an auth-gate response do not prove the
                // durable operation was rejected: a proxy can lose or replace
                // the response after the row is committed. Keep the exact
                // marker and reconcile through the operation endpoint.
                const rejectionIsDefinitive = res.status >= 400
                    && res.status < 500
                    && res.status !== 401
                    && res.status !== 408
                    && res.status !== 429;
                if (rejectionIsDefinitive) {
                    clearPanelCertificateMarker();
                    issueInFlightRef.current = false;
                    setPendingOperation(null);
                    const problem = await readApiError(res);
                    if (problem.code && problem.action) setRefusal(problem);
                    else showToast('error', apiErrorText(problem, t, 'panelCert.failed'));
                    setBusy(false);
                }
                return;
            }
            const operation = decodePanelCertificateOperation(await res.json(), marker);
            if (operation === null) throw new Error(t('panelCert.failed'));
        } catch (e) {
            // A lost POST response is not proof that the durable row was not
            // created. Keep the exact marker and let polling reconcile it.
            if (!(e instanceof TypeError)) {
                showToast('error', e instanceof Error && e.message ? e.message : t('panelCert.failed'));
            }
        }
    };

    const sameAddress = secureAddress !== '' && window.location.origin + '/' === secureAddress;
    const secureLink = (
        <a
            href={secureAddress}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 rounded-md border border-border-strong bg-surface px-3 py-1.5 text-sm font-medium text-fg transition-colors hover:bg-surface-2"
        >
            {t('panelCert.openSecure')}
        </a>
    );

    return (
        <section className="rounded-xl border border-border bg-surface p-6">
            <div className="mb-3 flex items-center gap-2">
                <Lock className="h-5 w-5 text-fg-subtle" />
                <h2 className="text-base font-semibold text-fg">{t('panelCert.title')}</h2>
            </div>

            {/* What is known comes first: the state of the certificate, or
                what became of the request. Then how to prepare, then what
                became of a request that is not settled, beside the form it
                was sent from.
                The state keeps its place and least height while it is read,
                when it could not be read and when it is known, so nothing
                under it moves when the answer arrives.
                Önce bilinen gelir: sertifikanın durumu ya da isteğin sonucu.
                Sonra hazırlık adımları, sonra da sonuçlanmamış isteğin
                durumu, gönderildiği formun yanında. Durum satırı üç durumda
                da yerini ve en az yüksekliğini korur. */}
            {outcome.state === 'issued' ? (
                <div ref={noticeRef} role="status" className="mb-5 scroll-mt-24 rounded-lg border border-border bg-surface-2/50 p-4 text-sm leading-relaxed text-fg">
                    <p className="flex items-start gap-2 font-semibold">
                        <BadgeCheck className="mt-0.5 h-4 w-4 shrink-0 text-success" aria-hidden="true" />
                        <span className="min-w-0 break-words">{t('panelCert.reopen.title', { domain: outcome.domain })}</span>
                    </p>
                    {reopening ? (
                        <>
                            <p className="mt-2 max-w-[75ch] break-words">
                                {sameAddress
                                    ? t('panelCert.reopen.reload', { seconds: secondsLeft })
                                    : t('panelCert.reopen.body', { domain: outcome.domain, address: secureAddress, seconds: secondsLeft })}
                            </p>
                            <Button type="button" className="mt-3" onClick={() => setStay(true)}>
                                {t('panelCert.reopen.stay')}
                            </Button>
                        </>
                    ) : (
                        <>
                            <p className="mt-2 max-w-[75ch] break-words">
                                {t('panelCert.reopen.stayed', { domain: outcome.domain, address: secureAddress })}
                            </p>
                            <div className="mt-3">{secureLink}</div>
                        </>
                    )}
                </div>
            ) : (
                <div className="mb-4 min-h-[6.75rem] sm:min-h-[4.5rem]">
                    <RemoteGate
                        remote={certificate.remote}
                        checking={t('panelCert.checking')}
                        failed={t('panelCert.unknown')}
                        onRetry={() => void certificate.retry()}
                        busy={certificate.reading}
                        className="py-3"
                    >
                        {({ value: info }) => (
                            <div className="flex items-start gap-2 rounded-lg border border-border bg-surface-2/50 p-3 text-sm">
                                {info.https_enabled && !info.self_signed ? (
                                    <BadgeCheck className="mt-0.5 h-4 w-4 shrink-0 text-success" aria-hidden="true" />
                                ) : (
                                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                                )}
                                <div className="min-w-0 text-fg-muted">
                                    {!info.https_enabled
                                        ? t('panelCert.notReadable')
                                        : info.self_signed
                                            ? t('panelCert.selfSigned')
                                            : t('panelCert.real', { issuer: info.issuer })}
                                    {info.https_enabled && !info.self_signed && info.expires_at && (
                                        <span className="block text-xs text-fg-subtle">
                                            {t('panelCert.expires', { date: new Date(info.expires_at).toLocaleDateString(locale) })}
                                        </span>
                                    )}
                                </div>
                            </div>
                        )}
                    </RemoteGate>
                </div>
            )}

            <ol className="mb-5 list-decimal space-y-2 pl-5 text-sm text-fg-muted">
                <li>{t('start.panel.dns')}</li>
                <li>{t('start.panel.check')}</li>
                <li>{t('start.panel.issue')}</li>
            </ol>

            {outcome.state !== 'issued' && (
                <>
                    <div ref={noticeRef} className="scroll-mb-24 scroll-mt-24">
                        {refusal && (
                            <div role="alert" data-panel-certificate-refusal={refusal.code}>
                                <ErrorBanner className="mb-4" error={refusal} />
                            </div>
                        )}
                        {outcome.state === 'failed' && (
                            <ErrorBanner
                                className="mb-4"
                                error={{
                                    message: outcome.reason
                                        ? t('panelCert.failedDetail', { domain: outcome.domain, reason: outcome.reason })
                                        : t('panelCert.failedPlain', { domain: outcome.domain }),
                                }}
                            />
                        )}
                        {outcome.state === 'notRecorded' && (
                            <div role="status" className="mb-4 rounded-lg border border-border bg-surface-2/50 p-3 text-sm leading-relaxed text-fg">
                                <p className="max-w-[75ch] break-words">{t('panelCert.notRecorded', { domain: outcome.domain })}</p>
                            </div>
                        )}
                        {pendingOperation !== null && unconfirmed && (
                            <CouldNotCheck
                                className="mb-4"
                                text={t('panelCert.unconfirmed', { domain: pendingOperation.domain, address: secureAddress })}
                                beside={secureLink}
                                actionLabel={t('panelCert.checkAgain')}
                                onRetry={() => setCheckNow((n) => n + 1)}
                            />
                        )}
                    </div>

                    <label htmlFor="panel-certificate-domain" className="mb-1 block text-xs font-medium text-fg-muted">
                        {t('panelCert.domain')}
                    </label>
                    <div className="flex flex-col gap-2 sm:flex-row">
                        <input
                            id="panel-certificate-domain"
                            value={pendingOperation?.domain ?? domain}
                            onChange={(e) => setDomain(e.target.value.trim())}
                            readOnly={pendingOperation !== null}
                            placeholder="panel.example.com"
                            autoComplete="url"
                            className={inputClass + ' min-w-0 flex-1'}
                        />
                        <Button
                            className="w-full sm:w-auto"
                            variant="primary"
                            onClick={issue}
                            disabled={!certificateKnown || busy || pendingOperation !== null || restarting || !domain}
                            loading={busy && !unconfirmed}
                        >
                            {busy && !unconfirmed ? t('panelCert.issuing') : t('panelCert.issue')}
                        </Button>
                    </div>
                    <p className="mt-2 text-xs text-fg-subtle">{t('panelCert.hint')}</p>
                </>
            )}
        </section>
    );
}
