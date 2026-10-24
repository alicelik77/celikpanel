import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { Settings, Play, StopCircle, RotateCw, RefreshCw, ScanSearch, DownloadCloud, ChevronDown, ChevronRight, Trash2, ShieldOff, Layers, Globe, Database, Mail, Network, Shield, Zap, FolderUp, Activity, Boxes, Clock } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { Button, Checking, CouldNotCheck, Dialog, EmptyState, ErrorBanner, KeepWords, SearchInput, Spinner, StatusDot } from './ui';
import { PageHeader } from './PageHeader';
import { readApiError, apiErrorText, type ApiError } from '../lib/apiError';
import { ServiceActionNotice, isServiceActionOutcome } from './ServiceActionNotice';
import { readServiceActionNote } from '../lib/serviceActionNote';
import { lastKnown, useRemote } from '../lib/remote';
import {
    canonicalMailHostname,
    decodeMailHostnameIdentity,
    decodeManagedMailProfiles,
    useComponentOperation,
    type MailHostnameIdentity,
    type ManagedMailProfile,
} from './ComponentOperation';
import { Link, useLocation, useNavigate } from '../router';
import { FirewallBar } from './FirewallPanel';
import { componentStateWord, isServiceRunning, phpBindingOf, phpNotReadyHintKey } from '../lib/serviceStatus';
import { decodeCandidateVersion, decodeRepoInfo, type RepoInfo } from '../lib/componentRepo';
import { decodeInstallPreview, installPreviewURL } from '../lib/installPreview';
import { decodeRoundcubeUpdate, ROUNDCUBE_UPDATE_URL } from '../lib/roundcubeUpdate';
import { InstallPreviewNotes, installPreviewAckKey, installPreviewGate } from './InstallPreviewNotes';
import { useAuth } from '../auth/AuthContext';
import type { TranslationKey } from '../i18n/en';
import { useServerSetup } from './ServerSetupGate';
import { decodeServerSetup, setupEditorKey, type ServerSetupSnapshot } from '../lib/serverSetup';
import { publishComponentCensus } from '../lib/componentCensus';
import { componentDescription, componentName } from '../lib/componentName';
import { mailProfileName, webmailReportsProblem } from '../lib/mailProfileText';
import { decodeReadyBundles, bundleChipState, bundleName, type ReadyBundle } from '../lib/readyBundles';
import { componentIcon } from '../lib/componentIcons';
import { packageVersionFacts } from '../lib/packageVersion';
import { BundleInstallDialog, ReadyBundleStrip, useBundleSequence, type BundleChoices } from './ReadyBundleStrip';
import { WebmailRepairButton } from './WebmailRepairButton';
import { answerBody, decodePHPToolMoveFailure, phpToolForService, readPHPTools, PHP_TOOL_SERVICE_ID, type PHPToolMoveFailure, type PHPToolsInfo, phpToolCauseText } from '../lib/phpTools';

// One installed copy of a runtime (B3b): php8.3-fpm is an instance, a Node
// tree under /opt/celikpanel/runtimes is an instance. `unit` empty means the
// copy has no daemon of its own — no status, no start/stop for it.
// Bir runtime'ın kurulu tek kopyası (B3b): php8.3-fpm bir kopyadır,
// /opt/celikpanel/runtimes altındaki bir Node ağacı bir kopyadır. `unit` boşsa
// kopyanın kendine ait daemon'ı yoktur — durumu da başlat/durduru da olmaz.
interface ServiceInstance {
    version: string;
    unit?: string;
    path?: string;
    managed: boolean;
    status?: string;
    size_bytes?: number;
}

interface ManagedService {
    id: string;
    /** The real systemd unit the scan found (BIND's id is "bind", unit "named"). */
    /** Taramanın bulduğu gerçek systemd unit'i (BIND id'si "bind", unit'i "named"). */
    unit?: string;
    name: string;
    description: string;
    icon: string;
    category: string;
    versions: string[];
    instances?: ServiceInstance[];
    /** "unknown" until this host has actually been observed. */
    /** Bu makine gerçekten gözlenene dek "unknown". */
    status: string;
    /**
     * Three answers, not two: true = observed present, false = observed
     * absent, null = this panel has never looked at this server for this
     * component. Null is NOT "no" — the screen must say "not checked yet" and
     * offer the check instead of reporting an inventory nobody took.
     *
     * Üç yanıt, iki değil: true = gözlendi ve var, false = gözlendi ve yok,
     * null = bu panel bu sunucuya bu bileşen için hiç bakmadı. Null "hayır"
     * DEĞİLDİR — ekran, kimsenin yapmadığı bir envanteri bildirmek yerine
     * "henüz bakılmadı" demeli ve bakmayı önermelidir.
     */
    is_installed: boolean | null;
    /** Which question the server answered: systemd unit, package, or app files. */
    /** Sunucunun hangi soruyu yanıtladığı: systemd unit'i, paket ya da uygulama dosyaları. */
    installed_evidence?: 'systemd_unit' | 'package' | 'application_files';
    conflict_with?: string;
    not_offered?: boolean;
    not_offered_kind?: 'integration' | 'distribution';
    not_offered_reason?: string;
    requires_missing?: string[];
    kind?: 'service' | 'runtime' | 'tool';
    packages?: string[];
    ports?: string[];
    /** Exact installed version package to reuse for a safe idempotent repair. */
    /** Güvenli ve idempotent onarımda yeniden kullanılacak tam kurulu sürüm paketi. */
    repair_package?: string;
    /** False when a versioned apt install cannot be tied to one exact package. */
    /** Sürümlü apt kurulumu tek bir kesin paketle eşleştirilemiyorsa false olur. */
    repair_available: boolean;
    /** alpha.104: the PHP version an installed PHP tool is served from. */
    /** Kurulu bir PHP aracının sunulduğu PHP sürümü. */
    php_binding?: { version: string; socket_ready: boolean; version_installed: boolean };
}

interface ManagedServicesSnapshot {
    services: ManagedService[];
    profiles: ManagedMailProfile[];
    // The ready bundles (alpha.94); an empty list from an older Panel or an
    // answer that is not the contract, so no strip offers an action then.
    // Hazır paketler; eski Panel'de ya da sözleşme dışı yanıtta boş liste.
    bundles: ReadyBundle[];
    scannedAt: string | null;
    // The scan's age on the server's clock at the answer; null from an older
    // server or with no scan. / Taramanın yanıt anında sunucu saatiyle yaşı;
    // eski sunucuda ya da tarama yoksa null.
    scanAgeSeconds: number | null;
    dnsIdentityReady: boolean;
    mailHostname: MailHostnameIdentity;
    mailDNSGuidance: MailDNSGuidance | null;
    // The Panel's record of a Roundcube install or removal that did not
    // finish (alpha.95, additive); null when there is none or from an older
    // Panel. Only then does the row offer the leftovers cleanup.
    // Bitmeyen Roundcube kurulumu ya da kaldırması kaydı; yoksa null.
    webmailCleanup: WebmailCleanupRecord | null;
}
type WebmailCleanupRecord = 'install_failed' | 'removal_incomplete';
const decodeWebmailCleanup = (value: unknown): WebmailCleanupRecord | null => (value === 'install_failed' || value === 'removal_incomplete' ? value : null);
// Where the mail record is created when this server is the secondary DNS
// server (cmd/panel mailProfileDNSGuidance, additive 2026-10-16). Every value
// is re-checked; anything unexpected drops only the guidance, never the page.
// Ikincil DNS sunucusunda posta kaydinin olusturulacagi yer.
interface MailDNSGuidance { primaryNameserver: string; primaryIP: string; recordName: string; recordValue: string; peerRecordChannel: boolean }
const guidanceName = (value: unknown) => typeof value === 'string' && value.length <= 253 && /^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z0-9-]+$/i.test(value) ? value : '';
const guidanceIPv4 = (value: unknown) => typeof value === 'string' && /^(\d{1,3}\.){3}\d{1,3}$/.test(value) ? value : '';
function decodeMailDNSGuidance(value: unknown): MailDNSGuidance | null {
    if (!value || typeof value !== 'object') return null;
    const payload = value as Record<string, unknown>;
    if (payload.reason !== 'dns_secondary' || payload.record_type !== 'A') return null;
    return {
        primaryNameserver: guidanceName(payload.primary_nameserver),
        primaryIP: guidanceIPv4(payload.primary_ip),
        recordName: guidanceName(payload.record_name),
        recordValue: guidanceIPv4(payload.record_value),
        // Additive (2026-10-16): an older server does not say, which reads as
        // no channel - the record is then added by hand, as before.
        peerRecordChannel: payload.peer_record_channel === true,
    };
}
type ServiceLifecycleAction = 'start' | 'stop' | 'restart';

interface HostMutationReadiness {
    ready: boolean;
    code?: 'HOST_MUTATION_BUSY' | 'HOST_MUTATION_UNAVAILABLE';
    reason?: 'panel_operation_active' | 'agent_mutation_active' | 'host_lock_busy' | 'package_manager_active' | 'state_unverified';
}

type PendingComponentAction =
    | { kind: 'service'; service: ManagedService; action: ServiceLifecycleAction }
    | { kind: 'instance'; service: ManagedService; unit: string; action: ServiceLifecycleAction }
    | { kind: 'repair'; service: ManagedService }
    | { kind: 'install-package'; service: ManagedService; pkg: string; version: string }
    | { kind: 'install-node'; service: ManagedService; version: string };

/**
 * Why the components' state is unverified (D-024: a screen does not claim
 * what did not happen).
 * - action: a component action was sent and the state after it could not be
 *   confirmed, so the action may have completed.
 * - read: no action was sent from this page; reading the state failed (the
 *   automatic check, a load or Rescan). `error` is that read's answer, kept
 *   in memory only; null when no answer came back or it could not be parsed.
 * An action's unknown outcome is never replaced by a later failed read.
 * Bileşen durumunun neden doğrulanmadığı. action: bir işlem gönderildi ve
 * sonrası doğrulanamadı. read: bu sayfa işlem göndermedi, durum okunamadı.
 * Bir işlemin bilinmeyen sonucu sonraki başarısız okumayla silinmez.
 */
type UnverifiedCause = { kind: 'action' } | { kind: 'read'; error: ApiError | null };

interface ServiceVerificationState {
    unverified: boolean;
    baselineScannedAt: string | null;
    cause: UnverifiedCause;
}

const unverifiedByAction: UnverifiedCause = { kind: 'action' };

const serviceVerificationStorageKey = 'celikpanel.components.unverified-state';

function isServiceInstance(value: unknown): value is ServiceInstance {
    if (!value || typeof value !== 'object') return false;
    const instance = value as Record<string, unknown>;
    return (
        typeof instance.version === 'string' &&
        typeof instance.managed === 'boolean' &&
        (instance.unit === undefined || typeof instance.unit === 'string') &&
        (instance.path === undefined || typeof instance.path === 'string') &&
        (instance.status === undefined || typeof instance.status === 'string') &&
        (instance.size_bytes === undefined || typeof instance.size_bytes === 'number')
    );
}

function isManagedService(value: unknown): value is ManagedService {
    if (!value || typeof value !== 'object') return false;
    const service = value as Record<string, unknown>;
    return (
        typeof service.id === 'string' &&
        typeof service.name === 'string' &&
        typeof service.description === 'string' &&
        typeof service.icon === 'string' &&
        typeof service.category === 'string' &&
        typeof service.status === 'string' &&
        // Null is a first-class answer here ("never observed"), so it is
        // accepted; anything else still fails the payload closed.
        // Null burada birinci sınıf bir yanıttır ("hiç gözlenmedi").
        (service.is_installed === null || typeof service.is_installed === 'boolean') &&
        Array.isArray(service.versions) &&
        service.versions.every((version) => typeof version === 'string') &&
        (service.instances === undefined ||
            (Array.isArray(service.instances) && service.instances.every(isServiceInstance))) &&
        (service.ports === undefined ||
            (Array.isArray(service.ports) && service.ports.every((port) => typeof port === 'string'))) &&
        (service.repair_available === undefined || typeof service.repair_available === 'boolean')
    );
}

// A snapshot is trusted only after validating the fields this screen uses.
// An array-shaped error or half-written payload must never unlock mutations.
// Bir snapshot yalnız bu ekranın kullandığı alanlar doğrulandıktan sonra
// güvenilir sayılır. Dizi görünümlü hata ya da yarım yük mutasyon kilidini
// asla açmamalıdır.
function parseManagedServicesSnapshot(value: unknown): ManagedServicesSnapshot | null {
    if (!value || typeof value !== 'object') return null;
    const payload = value as Record<string, unknown>;
    if (!Array.isArray(payload.services) || !payload.services.every(isManagedService)) return null;
    const serviceIDs = new Set<string>();
    for (const service of payload.services) {
        if (
            service.id.trim() !== service.id
            || service.id === ''
            || serviceIDs.has(service.id)
        ) {
            return null;
        }
        serviceIDs.add(service.id);
    }
    const profiles = decodeManagedMailProfiles(payload.profiles, serviceIDs);
    if (profiles === null) return null;
    const mailHostname = decodeMailHostnameIdentity(payload.mail_hostname);
    if (mailHostname === null) return null;
    if (
        typeof payload.dns_identity_ready !== 'boolean'
        ||
        payload.scanned_at !== undefined
        && payload.scanned_at !== null
        && (
            typeof payload.scanned_at !== 'string'
            || !Number.isFinite(Date.parse(payload.scanned_at))
        )
        // Optional; when present, a finite number next to a stamp.
        // İsteğe bağlı; varsa bir damganın yanında sonlu bir sayı.
        || payload.scan_age_seconds !== undefined
        && payload.scan_age_seconds !== null
        && (
            typeof payload.scan_age_seconds !== 'number'
            || !Number.isFinite(payload.scan_age_seconds)
            || typeof payload.scanned_at !== 'string'
        )
    ) {
        return null;
    }
    return {
        services: payload.services,
        profiles,
        bundles: decodeReadyBundles(payload.bundles, serviceIDs) ?? [],
        scannedAt: typeof payload.scanned_at === 'string' ? payload.scanned_at : null,
        scanAgeSeconds: typeof payload.scan_age_seconds === 'number' ? payload.scan_age_seconds : null,
        dnsIdentityReady: payload.dns_identity_ready,
        mailHostname,
        mailDNSGuidance: payload.dns_identity_ready ? null : decodeMailDNSGuidance(payload.mail_dns_guidance),
        webmailCleanup: decodeWebmailCleanup(payload.webmail_cleanup),
    };
}

function parseHostMutationReadiness(value: unknown): HostMutationReadiness | null {
    if (!value || typeof value !== 'object') return null;
    const payload = value as Record<string, unknown>;
    if (typeof payload.ready !== 'boolean') return null;
    if (payload.ready) {
        return payload.code === undefined && payload.reason === undefined ? { ready: true } : null;
    }
    if (payload.code !== 'HOST_MUTATION_BUSY' && payload.code !== 'HOST_MUTATION_UNAVAILABLE') return null;
    if (
        payload.reason !== 'panel_operation_active'
        && payload.reason !== 'agent_mutation_active'
        && payload.reason !== 'host_lock_busy'
        && payload.reason !== 'package_manager_active'
        && payload.reason !== 'state_unverified'
    ) return null;
    return {
        ready: false,
        code: payload.code,
        reason: payload.reason,
    };
}

function readServiceVerificationState(): ServiceVerificationState {
    const verified: ServiceVerificationState = { unverified: false, baselineScannedAt: null, cause: unverifiedByAction };
    if (typeof sessionStorage === 'undefined') return verified;
    try {
        const value = JSON.parse(sessionStorage.getItem(serviceVerificationStorageKey) ?? 'null') as Record<string, unknown> | null;
        if (!value || value.unverified !== true) return verified;
        return {
            unverified: true,
            baselineScannedAt: typeof value.baselineScannedAt === 'string' ? value.baselineScannedAt : null,
            // A record from before the cause was kept, or any other value,
            // keeps the action wording: the safer of the two claims.
            // Nedeni saklanmadan önceki kayıt işlem sözünü korur.
            cause: value.cause === 'read' ? { kind: 'read', error: null } : unverifiedByAction,
        };
    } catch {
        return verified;
    }
}

function persistServiceVerificationState(value: ServiceVerificationState) {
    if (typeof sessionStorage === 'undefined') return;
    try {
        // Only the kind of cause is kept across a reload, not the answer.
        // Yeniden yüklemede yalnız nedenin türü saklanır, yanıt değil.
        if (value.unverified) sessionStorage.setItem(serviceVerificationStorageKey, JSON.stringify({
            unverified: true, baselineScannedAt: value.baselineScannedAt, cause: value.cause.kind,
        }));
        else sessionStorage.removeItem(serviceVerificationStorageKey);
    } catch {
        // Restricted storage must not bypass the in-memory safety lock.
        // Kısıtlı depolama, bellek içindeki güvenlik kilidini aşmamalıdır.
    }
}

// A tarball runtime installs from upstream (nodejs.org), not from distro
// packages — its install box lives in the version drawer, not the package
// dialog. Data-driven: "runtime with no packages" is exactly that set.
// Tarball runtime, dağıtım paketinden değil kaynağından (nodejs.org) kurulur —
// kurulum kutusu paket modalında değil sürüm çekmecesindedir. Veriden türer:
// "paketi olmayan runtime" tam olarak o kümedir.
const isTarballRuntime = (s: ManagedService) => s.kind === 'runtime' && !(s.packages && s.packages.length > 0);

// "Nobody has looked" is its own row state, distinct from "we looked and it
// is not there". It reaches the screen from a server that has never been
// scanned — a fresh install, a restored host — and from a component this
// panel learned about after the last scan ran.
// "Kimse bakmadı", "baktık ve yok"tan ayrı bir satır durumudur.
const notChecked = (s: ManagedService) => s.is_installed === null;

// Requirement ROLE tokens (seat names — any member satisfies) and their
// localized labels. Shared by the row badges and the version drawer.
// Gereksinim ROL belirteçleri (koltuk adları — herhangi bir üye tatmin eder)
// ve yerel etiketleri. Satır rozetleri ile sürüm çekmecesinin ortak malı.
const REQ_ROLE_KEYS: Record<string, string> = {
    'web-server': 'services.role.webServer',
    'dns-server': 'services.role.dnsServer',
    'smtp-server': 'services.role.smtpServer',
    'imap-server': 'services.role.imapServer',
    'spam-filter': 'services.role.spamFilter',
    'kv-store': 'services.role.kvStore',
};

// Category display order + label key + section icon + icon tint. Each
// category renders as its own card; the colored icon chip is what makes the
// sections scannable at a glance in both themes.
// Kategori gösterim sırası + etiket anahtarı + bölüm ikonu + ikon rengi.
// Her kategori kendi kartı; renkli ikon rozeti bölümleri iki temada da tek
// bakışta taranabilir kılan şeydir.
// Ordered by necessity, not alphabet: the panel is authoritative DNS for its
// domains (D-009 — no DNS server, no domains), so DNS leads; then the hosting
// core (web, database, email), then hardening and extras.
// Alfabetik değil gereklilik sırası: panel domain'lerinin DNS otoritesidir
// (D-009 — DNS yoksa domain yok), bu yüzden DNS başta; sonra barındırma
// çekirdeği (web, veritabanı, e-posta), sonra sertleştirme ve ekstralar.
const categoryOrder: { id: string; labelKey: string; icon: LucideIcon; tint: string }[] = [
    { id: 'dns', labelKey: 'services.cat.dns', icon: Network, tint: 'bg-surface-2 text-fg-muted' },
    { id: 'web', labelKey: 'services.cat.web', icon: Globe, tint: 'bg-surface-2 text-fg-muted' },
    { id: 'database', labelKey: 'services.cat.database', icon: Database, tint: 'bg-surface-2 text-fg-muted' },
    { id: 'email', labelKey: 'services.cat.email', icon: Mail, tint: 'bg-surface-2 text-fg-muted' },
    { id: 'security', labelKey: 'services.cat.security', icon: Shield, tint: 'bg-surface-2 text-fg-muted' },
    { id: 'cache', labelKey: 'services.cat.cache', icon: Zap, tint: 'bg-surface-2 text-fg-muted' },
    { id: 'ftp', labelKey: 'services.cat.ftp', icon: FolderUp, tint: 'bg-surface-2 text-fg-muted' },
    { id: 'monitoring', labelKey: 'services.cat.monitoring', icon: Activity, tint: 'bg-surface-2 text-fg-muted' },
    { id: 'system', labelKey: 'services.cat.system', icon: Clock, tint: 'bg-surface-2 text-fg-muted' },
];

interface ServiceListProps {
    onSelectConfig: (path: string) => void;
    onManageService?: (serviceId: string, versions: string[]) => void;
}

// A cached scan keeps the page instant. Revalidate it in the background when
// it is older than this, and always scan on the first visit after installation.
// Önbellekteki tarama sayfayı anında açar. Bu süreden eskiyse arka planda
// doğrula; kurulumdan sonraki ilk ziyarette ise mutlaka tara.
const AUTO_SCAN_MAX_AGE_SECONDS = 300;

// A service's optional managed vendor repository (e.g. PGDG for PostgreSQL).
// When available+enabled, the install dialog offers a specific major version.
// Bir servisin isteğe bağlı yönetilen vendor deposu (örn. PostgreSQL için PGDG).
// Mevcut+etkinse, kurulum modalı belirli bir major sürüm sunar.

// unknownCategories returns a card definition for every category present in
// the data but missing from categoryOrder above, so nothing can be silently
// dropped. The label falls back to the raw category id — honest and obviously
// unfinished, which is exactly the signal that the id belongs in categoryOrder
// with a proper label and icon.
// unknownCategories, veride bulunup yukarıdaki categoryOrder'da olmayan her
// kategori için bir kart tanımı döndürür; böylece hiçbir şey sessizce
// düşmez. Etiket ham kategori id'sine düşer — dürüst ve bariz biçimde
// yarım; ki bu da o id'nin düzgün bir etiket ve ikonla categoryOrder'a
// girmesi gerektiğinin işaretidir.
function unknownCategories(list: ManagedService[]) {
    const known = new Set(categoryOrder.map((c) => c.id));
    const extra: string[] = [];
    for (const s of list) {
        if (s.category && !known.has(s.category) && !extra.includes(s.category)) extra.push(s.category);
    }
    return extra.map((id) => ({ id, labelKey: id, icon: Boxes, tint: 'bg-surface-2 text-fg-muted' }));
}

// Services grouped into per-category cards (Claude Design'dan uyarlandı).
// Status comes straight from managed-services (the reliable source:
// "active (running)" vs "inactive (dead)"), not the wrapper-unit endpoint
// that misreports.
//
// Servisler kategori başına kartlar halinde. Durum doğrudan
// managed-services'ten gelir (güvenilir kaynak), yanlış raporlayan
// wrapper-unit uç noktasından değil.
export function ServiceList({ onManageService }: ServiceListProps) {
    const { t } = useI18n();
    const navigate = useNavigate();
    const location = useLocation();
    const componentOperation = useComponentOperation();
    const { startInstall, catalogSnapshot } = componentOperation;
    const [services, setServices] = useState<ManagedService[]>([]);
    const [profiles, setProfiles] = useState<ManagedMailProfile[]>([]);
    const [bundles, setBundles] = useState<ReadyBundle[]>([]);
    const [bundleTarget, setBundleTarget] = useState<ReadyBundle | null>(null);
    const [webmailCleanup, setWebmailCleanup] = useState<WebmailCleanupRecord | null>(null);
    const [profileRepair, setProfileRepair] = useState(false);
    // A mail bundle whose chip says Install opens the dialogue in its install
    // form, even when the Panel calls the profile partial only because an
    // installed bundle shares its parts (alpha.95 review).
    // Kur diyen posta çipi pencereyi kurulum biçiminde açar.
    const [profileFreshInstall, setProfileFreshInstall] = useState(false);
    const webmailProblem = webmailReportsProblem(profiles);
    const [scannedAt, setScannedAt] = useState<string | null>(null);
    // Counts the scans this page has received. The firewall banner reads its
    // state again with each, so it does not keep an Agent error the scan no
    // longer shows (sixteenth record, O5).
    // Bu sayfanin aldigi taramalari sayar; guvenlik duvari bandi her birinde
    // durumunu yeniden okur.
    const [scanRound, setScanRound] = useState(0);
    const [dnsIdentityReady, setDNSIdentityReady] = useState(false);
    const [mailHostname, setMailHostname] = useState<MailHostnameIdentity | null>(null);
    const [mailDNSGuidance, setMailDNSGuidance] = useState<MailDNSGuidance | null>(null);
    const serverSetup = useServerSetup();
    const { user } = useAuth();
    // A server whose guided setup has finished (or is being extended, with no
    // run active) adds mail through setup, so mail gets setup's steps and
    // checks: public DNS of the mail host name, the trusted certificate and its
    // renewal, reverse DNS and port 25 (2026-10-16).
    // Kurulumu bitmis sunucuda posta kurulum uzerinden eklenir.
    const setupSnapshot = serverSetup?.snapshot ?? null;
    const mailThroughSetup = !!setupSnapshot && !['running', 'waiting'].includes(setupSnapshot.status)
        && (setupSnapshot.status === 'ready' || setupSnapshot.extension === true);
    const [hostMutationReadiness, setHostMutationReadiness] = useState<HostMutationReadiness | null>(null);
    const [loading, setLoading] = useState(true);
    const [scanning, setScanning] = useState(false);
    // Opening this page runs a host-wide check when the cache is missing or
    // stale — this IS the inventory page, and looking is what it is for. But
    // an automatic check that fails leaves the list saying "not checked yet"
    // with no trace of the attempt, which reads as "nobody has pressed the
    // button" rather than "the panel tried and could not reach the host".
    // The distinction is the operator's next move, so the page keeps it.
    // Bu sayfayı açmak, önbellek yoksa ya da bayatsa sistem geneli bir kontrol
    // çalıştırır — burası envanter sayfasıdır, bakmak zaten işidir. Ama
    // başarısız otomatik kontrol, listeyi denemeden hiç iz bırakmadan "henüz
    // bakılmadı"da bırakır; bu da "kimse düğmeye basmadı" gibi okunur, oysa
    // doğrusu "panel denedi ve makineye ulaşamadı"dır. Fark operatörün bir
    // sonraki hamlesidir; sayfa onu saklar.
    const [autoCheckFailed, setAutoCheckFailed] = useState(false);
    const [busy, setBusy] = useState<string | null>(null);
    // What a start, stop or restart ended with when it is not "done": a
    // verified failure or an unknown result, with the service's own line. It
    // stays above the list until it is closed or another action is taken.
    // Bir başlat, durdur ya da yeniden başlat işleminin "yapıldı" olmayan
    // sonucu; kapatılana ya da başka bir işlem yapılana dek listenin üstünde.
    const [actionOutcome, setActionOutcome] = useState<ApiError | null>(null);
    const [installTarget, setInstallTarget] = useState<ManagedService | null>(null);
    const [profileTarget, setProfileTarget] = useState<ManagedMailProfile | null>(null);
    const [uninstallTarget, setUninstallTarget] = useState<ManagedService | null>(null);
    const [pendingAction, setPendingAction] = useState<PendingComponentAction | null>(null);
    const [verification, setVerification] = useState<ServiceVerificationState>(readServiceVerificationState);
    const latestScannedAtRef = useRef<string | null>(null);
    const stateUnverified = verification.unverified;
    // The same flag, set the moment it changes rather than on the next
    // render: a gate read between two bundle steps sees a flag set a moment
    // ago (alpha.94 review item 1).
    // Aynı bayrak, bir sonraki çizimde değil değiştiği anda kurulur.
    const stateUnverifiedRef = useRef(verification.unverified);

    const markStateUnverified = (cause: UnverifiedCause = unverifiedByAction) => {
        stateUnverifiedRef.current = true;
        setVerification((previous) => {
            const next: ServiceVerificationState = {
                unverified: true,
                baselineScannedAt: previous.unverified ? previous.baselineScannedAt : scannedAt,
                cause: previous.unverified && previous.cause.kind === 'action' ? previous.cause : cause,
            };
            persistServiceVerificationState(next);
            return next;
        });
    };

    const clearStateUnverified = () => {
        const next: ServiceVerificationState = { unverified: false, baselineScannedAt: null, cause: unverifiedByAction };
        stateUnverifiedRef.current = false;
        persistServiceVerificationState(next);
        setVerification(next);
    };

    const applySnapshot = (value: unknown, source: 'load' | 'scan'): boolean => {
        const snapshot = parseManagedServicesSnapshot(value);
        if (!snapshot) return false;
        if (source === 'scan' && snapshot.scannedAt === null) return false;
        if (source === 'scan') setScanRound((round) => round + 1);

        const currentScannedAt = latestScannedAtRef.current;
        const currentTimestamp = Date.parse(currentScannedAt || '');
        const nextTimestamp = Date.parse(snapshot.scannedAt || '');
        if (
            Number.isFinite(currentTimestamp)
            && (
                !Number.isFinite(nextTimestamp)
                || nextTimestamp < currentTimestamp
                || (source === 'load' && nextTimestamp === currentTimestamp)
            )
        ) {
            return true;
        }
        latestScannedAtRef.current = snapshot.scannedAt;
        // The sidebar badge reads this. Publishing from the single place this
        // page accepts a payload means the badge cannot disagree with the
        // rows below it, and cannot lag them by a page load either.
        // Kenar çubuğu rozeti bunu okur. Bu sayfanın yükü kabul ettiği tek
        // yerden yayınlamak, rozetin altındaki satırlarla çelişmemesini ve
        // onlardan bir sayfa yüklemesi geride kalmamasını sağlar.
        publishComponentCensus(snapshot.services);
        setServices(snapshot.services);
        setProfiles(snapshot.profiles);
        setBundles(snapshot.bundles);
        setWebmailCleanup(snapshot.webmailCleanup);
        setScannedAt(snapshot.scannedAt);
        setDNSIdentityReady(snapshot.dnsIdentityReady);
        setMailHostname(snapshot.mailHostname);
        setMailDNSGuidance(snapshot.mailDNSGuidance);

        // A cached load may still be the exact pre-mutation snapshot retained
        // after an atomic scan failure. Only a fresh scan, or a load carrying
        // a newer scan token, may release the mutation lock.
        // Önbellek yüklemesi, atomik tarama hatasından sonra korunan mutasyon
        // öncesi snapshot'ın aynısı olabilir. Kilidi yalnız taze tarama veya
        // daha yeni tarama belirteci taşıyan yükleme açabilir.
        const loadProvesNewState =
            source === 'load' &&
            snapshot.scannedAt !== null &&
            snapshot.scannedAt !== verification.baselineScannedAt;
        if (source === 'scan' || !stateUnverified || loadProvesNewState) clearStateUnverified();
        return true;
    };

    const handleStateRefreshFailure = (error: ApiError) => {
        markStateUnverified();
        showToast('error', apiErrorText(error, t, 'services.actionFailed'));
    };
    // Installed-first (D-010, Jul 18): the page shows what this server RUNS,
    // not the whole catalog. Page length then tracks the installed count, not
    // the catalog size — a clean server is 3-4 rows whether the catalog holds
    // 19 items or 40. This is the constitution's "a service that isn't
    // installed is invisible" as mechanics instead of a checkbox; the catalog
    // is the same list with the filter off, not a second screen.
    // Kurulu-önce (D-010, 18 Tem): sayfa, sunucunun ÇALIŞTIRDIĞINI gösterir,
    // tüm kataloğu değil. Böylece sayfa uzunluğu katalog boyutunu değil kurulu
    // kalem sayısını izler — temiz sunucu, katalog 19 da olsa 40 da olsa 3-4
    // satırdır. Bu, anayasanın "kurulu olmayan görünmezdir" maddesinin onay
    // kutusu değil mekanik hâli; katalog, aynı listenin süzgeci kapalı hâlidir,
    // ikinci ekran değil.
    // …with one correction from the field (operator, Jul 23: "hide not
    // installed keeps coming back checked"): installed-first is the right
    // FIRST impression, not a standing rule. The choice is now remembered,
    // and the catalog is reachable by a labelled button instead of a
    // checkbox — a filter that hides the only path to installing is not a
    // filter, it is a hidden step.
    // …sahadan gelen bir düzeltmeyle (operatör, 23 Tem: "hide not installed
    // sürekli seçili geliyor"): kurulu-önce doğru İLK izlenimdir, kalıcı bir
    // kural değil. Seçim artık hatırlanıyor ve kataloğa onay kutusu yerine
    // adı yazan bir düğmeyle ulaşılıyor — kurulumun tek yolunu gizleyen bir
    // süzgeç, süzgeç değil gizli bir adımdır.
    const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
    const [query, setQuery] = useState('');
    const viewKey = 'celikpanel.components.view';
    // The first visit opens the installed view (D-010). Only an explicit,
    // remembered "catalog" opens the catalogue; a missing, unknown or
    // unreadable choice is a first visit (alpha.100: it used to open the
    // catalogue whenever nothing was stored).
    // İlk ziyaret Kurulu görünümü açar (D-010). Kataloğu yalnız hatırlanan
    // açık bir "catalog" seçimi açar; olmayan, bilinmeyen ya da okunamayan
    // seçim ilk ziyarettir.
    const [hideNotInstalled, setHideNotInstalled] = useState(() => {
        try {
            return typeof localStorage === 'undefined' || localStorage.getItem(viewKey) !== 'catalog';
        } catch {
            return true;
        }
    });

    // Collapse follows the view, not a fixed default: the installed list is
    // short and wants to be readable at a glance; the full catalog is long
    // and wants folding (D-010 correction, learned from implementing it).
    // Katlama görünümü izler, sabit varsayılanı değil: kurulu liste kısadır ve
    // tek bakışta okunmak ister; tam katalog uzundur ve katlanmak ister
    // (D-010 düzeltmesi, uygulamadan öğrenildi).
    const setView = (installedOnly: boolean) => {
        setHideNotInstalled(installedOnly);
        try {
            localStorage.setItem(viewKey, installedOnly ? 'installed' : 'catalog');
        } catch {
            /* private mode / kısıtlı depolama — görünüm yine değişir */
        }
        setCollapsed(installedOnly ? new Set() : new Set(categoryOrder.map((c) => c.id)));
    };

    const loadHostMutationReadiness = async (): Promise<boolean> => {
        try {
            const response = await fetch('/api/v1/host-mutation-readiness', {
                cache: 'no-store',
            });
            if (!response.ok) {
                setHostMutationReadiness({
                    ready: false,
                    code: 'HOST_MUTATION_UNAVAILABLE',
                    reason: 'state_unverified',
                });
                return false;
            }
            const readiness = parseHostMutationReadiness(await response.json());
            if (!readiness) {
                setHostMutationReadiness({
                    ready: false,
                    code: 'HOST_MUTATION_UNAVAILABLE',
                    reason: 'state_unverified',
                });
                return false;
            }
            setHostMutationReadiness(readiness);
            return readiness.ready;
        } catch {
            setHostMutationReadiness({
                ready: false,
                code: 'HOST_MUTATION_UNAVAILABLE',
                reason: 'state_unverified',
            });
            return false;
        }
    };

    useEffect(() => {
        void loadServices();
        void loadHostMutationReadiness();
        const timer = window.setInterval(() => {
            void loadHostMutationReadiness();
        }, 5000);
        return () => window.clearInterval(timer);
    }, []);

    // The operation controller keeps the panel locked through its mandatory
    // post-install scan. Consume that exact fresh snapshot before the overlay
    // is removed instead of racing it with another independent load.
    // İşlem denetleyicisi, zorunlu kurulum sonrası tarama boyunca paneli kilitli
    // tutar. Overlay'i kaldırmadan önce, başka bağımsız yüklemeyle yarıştırmak
    // yerine bu kesin taze snapshot'ı kullan.
    useLayoutEffect(() => {
        if (!catalogSnapshot) return;
        if (!applySnapshot(catalogSnapshot, 'scan')) {
            markStateUnverified();
            showToast('error', t('services.scanFailed'));
        }
        setLoading(false);
    }, [catalogSnapshot]);

    // The Dashboard's Ready bundles card opens this page at its bundles
    // (alpha.94): every category is opened so each strip is visible, and the
    // list is scrolled to and focused once.
    // Panel'in Hazır paketler kartı bu sayfayı paketlerde açar.
    // The old address of the mail stacks (#mail-stacks, in bookmarks and
    // older links) is an alias: it scrolls to the email strip.
    // Eski posta yığınları adresi bir takma addır: e-posta şeridine kaydırır.
    const bundlesFocusedRef = useRef(false);
    useLayoutEffect(() => {
        if ((location.hash !== '#bundles' && location.hash !== '#mail-stacks') || loading || bundles.length === 0 || bundlesFocusedRef.current) return;
        const list = document.getElementById('bundles');
        if (!list) return;
        bundlesFocusedRef.current = true;
        setCollapsed(new Set());
        const target = location.hash === '#mail-stacks'
            ? (document.querySelector('[data-bundle-strip="email"]') as HTMLElement | null) ?? list
            : list;
        target.scrollIntoView({ block: 'start' });
        list.focus({ preventScroll: true });
    }, [loading, location.hash, bundles.length]);

    // Client-side filter: text search over name/description/id plus the
    // "hide not installed" switch. A live search overrides collapse so
    // matches are never hidden inside a folded group.
    // İstemci tarafı filtre: ad/açıklama/id üzerinde arama + "kurulu
    // olmayanları gizle" anahtarı. Aktif arama katlamayı geçersiz kılar;
    // eşleşme katlanmış grup içinde saklı kalmaz.
    const q = query.trim().toLowerCase();
    // The desktop version column comes back only when the package database
    // gave at least one installed component a version (alpha.97, D-036);
    // otherwise the version stays under the description, as in alpha.95, and
    // there is no column of empty cells.
    // Masaüstü sürüm sütunu yalnız en az bir kurulu bileşenin paket sürümü
    // biliniyorsa döner; yoksa sürüm açıklamanın altında kalır.
    const versionColumn = services.some((s) => packageVersionFacts(s) !== null);
    const filtered = services.filter(
        (s) =>
            // The installed view hides what we KNOW is absent. A component
            // nobody has checked is not known to be absent, so hiding it here
            // would turn "not checked" back into "not installed" — the exact
            // claim this page must stop making.
            // Kurulu görünümü, YOK olduğunu BİLDİĞİMİZİ gizler.
            // Roundcube's leftovers offer stays in sight: since alpha.100 the
            // first visit opens this view, and the Panel's record of an
            // unfinished install or removal must not hide behind the switch.
            // Roundcube kalıntı teklifi görünür kalır; ilk ziyaret artık bu
            // görünümü açar ve Panel kaydı düğmenin arkasında saklanmamalı.
            (!hideNotInstalled || s.is_installed !== false || (s.id === 'roundcube' && webmailCleanup !== null)) &&
            (q === '' || `${componentName(s, t)} ${s.name} ${s.description} ${componentDescription(s, t)} ${s.id}`.toLowerCase().includes(q)),
    );

    // A server nobody has looked at yet: every row is unknown. This is the
    // ordinary first state of a fresh or restored host, not a fault, so it
    // gets a plain explanation and the check itself — not an alarm.
    // Henüz kimsenin bakmadığı sunucu: her satır bilinmiyor. Taze ya da geri
    // yüklenmiş makinenin olağan ilk durumudur; alarm değil, düz bir açıklama
    // ve bakma eyleminin kendisi verilir.
    const hostNeverChecked = services.length > 0 && services.every(notChecked);

    const toggleGroup = (cat: string) =>
        setCollapsed((prev) => {
            const next = new Set(prev);
            next.has(cat) ? next.delete(cat) : next.add(cat);
            return next;
        });
    const visibleCats = () => categoryOrder.filter(({ id }) => filtered.some((s) => s.category === id));
    const collapseAll = () => setCollapsed(new Set(visibleCats().map((c) => c.id)));
    const expandAll = () => setCollapsed(new Set());

    // Load the cached snapshot first so returning visits stay instant. A
    // missing or stale snapshot is then revalidated through a conditional
    // scan: the backend coalesces StrictMode, multiple tabs and simultaneous
    // page opens into at most one host-wide probe.
    // Önce önbellekteki snapshot'ı yükle; geri dönen ziyaretler anında açılsın.
    // Eksik veya bayat snapshot daha sonra koşullu taramayla doğrulanır:
    // backend StrictMode'u, çoklu sekmeleri ve eşzamanlı açılışları en fazla
    // tek sistem geneli yoklamada birleştirir.
    const loadServices = async (
        { markUnverifiedOnFailure = true }: { markUnverifiedOnFailure?: boolean } = {},
    ): Promise<boolean> => {
        // A failed read: no action was sent here. A caller that sent one marks
        // it after this, and the action's wording wins.
        // Başarısız okuma: burada işlem gönderilmedi. İşlem gönderen çağıran
        // bundan sonra işaretler; işlem sözü kazanır.
        const failVerification = (error: ApiError | null = null): false => {
            if (markUnverifiedOnFailure) markStateUnverified({ kind: 'read', error });
            return false;
        };
        setLoading(true);
        try {
            const res = await fetch('/api/v1/managed-services');
            // A stored record the server cannot verify is not a failed read
            // to report: the page replaces it with its own conditional check
            // below, as it does a stale one (D-032, 2026-10-15). Any other
            // refusal is reported.
            // Sunucunun doğrulayamadığı kayıtlı kayıt bildirilecek bir okuma
            // hatası değildir: sayfa onu, bayat kayıt gibi, aşağıdaki koşullu
            // kontrolüyle kendisi yeniler.
            let storedScannedAt: string | null = null;
            if (!res.ok) {
                const error = await readApiError(res);
                if (error.code !== 'SERVICE_STATE_UNVERIFIED') {
                    showToast('error', apiErrorText(error, t));
                    return failVerification(error);
                }
            } else {
                const data: unknown = await res.json();
                const snapshot = parseManagedServicesSnapshot(data);
                if (!snapshot || !applySnapshot(data, 'load')) {
                    showToast('error', t('services.scanFailed'));
                    return failVerification();
                }

                // The server's own age of the scan decides, as it does for the
                // conditional scan, so the browser's clock is not compared with
                // the server's; a negative age (a stamp after the server's clock)
                // is not fresh there either. An older server sends no age: then
                // the stamp is compared with this browser's clock as before.
                // Taramanın sunucunun kendi saatiyle yaşı karar verir (koşullu
                // taramada olduğu gibi); tarayıcı saati sunucununkiyle
                // karşılaştırılmaz. Eksi yaş orada da taze değildir. Eski sunucu
                // yaş göndermez: o zaman damga eskisi gibi tarayıcı saatiyle
                // karşılaştırılır.
                const scannedTimestamp = Date.parse(snapshot.scannedAt ?? '');
                const cacheIsStale = snapshot.scanAgeSeconds !== null
                    ? snapshot.scanAgeSeconds < 0 || snapshot.scanAgeSeconds > AUTO_SCAN_MAX_AGE_SECONDS
                    : !Number.isFinite(scannedTimestamp)
                        || Date.now() - scannedTimestamp > AUTO_SCAN_MAX_AGE_SECONDS * 1000;
                if (!cacheIsStale) return true;
                storedScannedAt = snapshot.scannedAt;
            }

            // Keep a valid cached page visible while it is refreshed. A fresh
            // install has no trusted observation yet, so retain the loader
            // until the first scan completes instead of briefly claiming that
            // every catalogue item is not installed.
            // Geçerli önbelleği yenilerken sayfada tut. Taze kurulumda henüz
            // güvenilir gözlem yoktur; bütün katalog yanlışlıkla "kurulu değil"
            // görünmesin diye ilk tarama bitene dek yükleyiciyi koru.
            if (storedScannedAt !== null) setLoading(false);
            setScanning(true);
            // A toast is the wrong home for this: it leaves the screen while
            // the state it explains stays. The banner below outlives it.
            // Bunun yeri bir toast değildir: açıkladığı durum kalırken kendisi
            // ekrandan gider. Aşağıdaki şerit ondan uzun yaşar.
            const failAutoCheck = (error: ApiError | null = null): false => {
                setAutoCheckFailed(true);
                return failVerification(error);
            };
            try {
                const scanResponse = await fetch(
                    `/api/v1/managed-services/scan?max_age_seconds=${AUTO_SCAN_MAX_AGE_SECONDS}`,
                    { method: 'POST', cache: 'no-store' },
                );
                if (!scanResponse.ok) {
                    const error = await readApiError(scanResponse);
                    showToast('error', apiErrorText(error, t, 'services.scanFailed'));
                    return failAutoCheck(error);
                }
                const refreshed: unknown = await scanResponse.json();
                if (!applySnapshot(refreshed, 'scan')) {
                    showToast('error', t('services.scanFailed'));
                    return failAutoCheck();
                }
            } catch {
                showToast('error', t('services.scanFailed'));
                return failAutoCheck();
            } finally {
                setScanning(false);
            }
            return true;
        } catch {
            showToast('error', t('common.error'));
            return failVerification();
        } finally {
            setLoading(false);
        }
    };

    const scan = async (): Promise<boolean> => {
        setScanning(true);
        setAutoCheckFailed(false);
        try {
            const res = await fetch('/api/v1/managed-services/scan', {
                method: 'POST',
                cache: 'no-store',
            });
            // Rescan only reads: a failure is a failed read, never an action
            // that may have completed.
            // Yeniden tarama yalnız okur: başarısızlığı okunamamadır, tamamlanmış
            // olabilecek bir işlem değildir.
            if (!res.ok) {
                const error = await readApiError(res);
                showToast('error', apiErrorText(error, t, 'services.scanFailed'));
                markStateUnverified({ kind: 'read', error });
                return false;
            }
            const data: unknown = await res.json();
            if (!applySnapshot(data, 'scan')) {
                showToast('error', t('services.scanFailed'));
                markStateUnverified({ kind: 'read', error: null });
                return false;
            }
            return true;
        } catch {
            showToast('error', t('services.scanFailed'));
            markStateUnverified({ kind: 'read', error: null });
            return false;
        } finally {
            setScanning(false);
        }
    };

    // Version drawers open per runtime row (B3b). Independent of category
    // collapse: a drawer is row detail, not a group.
    // Sürüm çekmeceleri runtime satırı başına açılır (B3b). Kategori
    // katlamasından bağımsız: çekmece grup değil, satır ayrıntısıdır.
    const [openDrawers, setOpenDrawers] = useState<Set<string>>(new Set());
    const toggleDrawer = (id: string) =>
        setOpenDrawers((prev) => {
            const next = new Set(prev);
            next.has(id) ? next.delete(id) : next.add(id);
            return next;
        });

    // Install a not-yet-installed catalogue service on demand. The agent
    // installs exactly the whitelisted packages for this id, then enables the
    // unit. This is the one-click install the VPN page (and others) point to.
    // Kurulu-olmayan bir katalog servisini talep üzerine kur. Agent bu id
    // için yalnız beyaz-listedeki paketleri kurar, sonra unit'i etkinleştirir.
    // Actual install, run after the confirmation modal. The agent installs
    // exactly the whitelisted packages for this id, then enables the unit.
    // Gerçek kurulum, onay modalından sonra koşar. Agent bu id için yalnız
    // beyaz-listedeki paketleri kurar, sonra unit'i etkinleştirir.
    const doInstall = async (service: ManagedService, pkg?: string, confirmed: string[] = []) => {
        if (stateUnverified) {
            showToast('error', t('services.stateUnverifiedHint'));
            return;
        }
        setInstallTarget(null);
        await startInstall({
            serviceId: service.id,
            name: componentName(service, t),
            ...(pkg ? { package: pkg } : {}),
            // alpha.105: what the install brings is checked again on the
            // server against what the owner confirmed (Roundcube is a
            // verified tarball, not a package install).
            ...(service.id !== 'roundcube' ? { checkInstalls: true, confirmInstalls: confirmed } : {}),
        });
    };

    // Remove an installed service: stop + disable + purge via the agent, then
    // rescan. Shrinks the attack surface back down — the mirror of install.
    // The dialog stays OPEN through the request (B3d): a refusal renders its
    // evidence — who blocks — right where the decision is being made. It used
    // to close first, so a 409 could only flash as a toast.
    // Kurulu bir servisi kaldır: agent ile durdur + devre dışı + purge, sonra
    // yeniden tara. Saldırı yüzeyini geri küçültür — kurulumun aynası.
    // Modal istek boyunca AÇIK kalır (B3d): ret, kanıtını — kimin
    // engellediğini — kararın verildiği yerde çizer. Eskiden önce kapanıyordu;
    // 409 ancak toast olarak parlayıp sönebiliyordu.
    const [uninstallError, setUninstallError] = useState<ApiError | null>(null);
    const [retryCleanup, setRetryCleanup] = useState(false);
    const [cleanupAttempt, setCleanupAttempt] = useState<'initial' | 'retry' | null>(null);
    const openUninstallDialog = (service: ManagedService) => {
        setUninstallTarget(service);
        setUninstallError(null);
        setRetryCleanup(false);
        setCleanupAttempt(null);
    };
    // The row's remove button asks which version first when a runtime has
    // several (alpha.106, owner's question on Frankfurt: "which version would
    // it remove?"); one version, or none, goes straight to the removal, whose
    // title names it.
    // Birden çok sürümde satırın kaldır düğmesi önce hangi sürüm olduğunu sorar.
    const [versionChoice, setVersionChoice] = useState<ManagedService | null>(null);
    const [versionRemoveRequest, setVersionRemoveRequest] = useState<{ serviceId: string; version: string; token: number } | null>(null);
    const openRemovalFlow = (service: ManagedService) => {
        setUninstallTarget(null);
        if (removableVersions(service).length > 1) {
            setVersionChoice(service);
            return;
        }
        openUninstallDialog(service);
    };
    const removeComponentById = (id: string) => {
        const target = services.find((candidate) => candidate.id === id);
        setUninstallTarget(null);
        if (target && target.is_installed === true) openRemovalFlow(target);
    };
    const chooseVersionRemoval = (service: ManagedService, version: string) => {
        setVersionChoice(null);
        setOpenDrawers((prev) => new Set(prev).add(service.id));
        setVersionRemoveRequest({ serviceId: service.id, version, token: Date.now() });
    };
    const openWebmailCleanupDialog = (service: ManagedService) => {
        setUninstallTarget(service);
        setUninstallError(null);
        setRetryCleanup(true);
        setCleanupAttempt(null);
    };
    // When the row offers the leftovers cleanup: the Panel's record says so.
    // Since alpha.96 the Panel folds the webmail profile's last attempt into
    // that record (the newer of the Roundcube entry and the webmail attempt
    // decides), so a later clean removal ends an offer a failed webmail
    // attempt raised. Otherwise the cleanup is reached from Roundcube's own
    // page, which opens this page with ?cleanup=roundcube.
    // Satır temizliği yalnız Panel kaydı varken sunar; kayıt Roundcube girdisi
    // ile webmail denemesinin yenisidir. Yoksa Roundcube'un kendi sayfası bu
    // sayfayı ?cleanup=roundcube ile açar.
    const webmailCleanupOffered = webmailCleanup !== null;
    const webmailCleanupNote: 'removal' | 'install' = webmailCleanup === 'removal_incomplete' ? 'removal' : 'install';
    const cleanupLinkRef = useRef(false);
    useEffect(() => {
        if (cleanupLinkRef.current || loading || !new URLSearchParams(location.search).has('cleanup')) return;
        if (new URLSearchParams(location.search).get('cleanup') !== 'roundcube') return;
        const roundcube = services.find((service) => service.id === 'roundcube');
        if (!roundcube || roundcube.is_installed !== false) return;
        cleanupLinkRef.current = true;
        openWebmailCleanupDialog(roundcube);
        // The link has done its work: the address loses ?cleanup so a reload
        // does not open the confirmation again (alpha.96). Replace, not push.
        // Bağlantı işini yaptı: yeniden yükleme onayı tekrar açmasın diye
        // adresten ?cleanup kaldırılır.
        const remaining = new URLSearchParams(location.search);
        remaining.delete('cleanup');
        const query = remaining.toString();
        navigate(location.pathname + (query ? `?${query}` : '') + location.hash, { replace: true });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [loading, location.search, services]);
    const closeUninstallDialog = () => {
        setUninstallTarget(null);
        setUninstallError(null);
        setRetryCleanup(false);
        setCleanupAttempt(null);
    };
    const doUninstall = async (service: ManagedService) => {
        if (stateUnverified) {
            showToast('error', t('services.stateUnverifiedHint'));
            return;
        }
        // Capture whether the operator clicked the post-error Retry action before
        // clearing that error. The busy label must describe this request for its
        // entire lifetime, including the verified-cache reload below.
        const nextCleanupAttempt = retryCleanup
            ? (uninstallError ? 'retry' : 'initial')
            : null;
        setCleanupAttempt(nextCleanupAttempt);
        setBusy(service.id);
        setUninstallError(null);
        try {
            const res = await fetch('/api/v1/service/uninstall', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ service_id: service.id }),
            });
            if (!res.ok) {
                const error = await readApiError(res);
                if (error.code === 'SERVICE_STATE_REFRESH_FAILED') {
                    closeUninstallDialog();
                    handleStateRefreshFailure(error);
                    return;
                }
                // Package removal has already succeeded, but the idempotent webmail
                // cleanup still needs another attempt. Keep this stale target only as
                // the retry handle while the verified cache updates the service row.
                if (error.code === 'WEBMAIL_UNINSTALL_PARTIAL') {
                    // An ordinary uninstall can enter cleanup mode here. It is still
                    // the initial cleanup attempt, never a user-requested retry.
                    if (nextCleanupAttempt === null) setCleanupAttempt('initial');
                    const verified = await loadServices({ markUnverifiedOnFailure: false });
                    if (!verified) {
                        // Closing also resets the stale retry error/label. Do this
                        // before the fail-closed Rescan lock disables the fieldset,
                        // otherwise the operator can be trapped in this modal.
                        closeUninstallDialog();
                        markStateUnverified();
                        return;
                    }
                    // The partial-success copy directs the operator back to this
                    // dialog, so expose it only after a verified reload proves the
                    // dialog can safely stay open and its Retry action is reachable.
                    setUninstallError(error);
                    setRetryCleanup(true);
                    showToast('error', apiErrorText(error, t, 'services.actionFailed'));
                    return;
                }
                // The removal and fresh scan succeeded; reload that verified cache while
                // keeping the firewall warning visible instead of retaining the old row.
                // Kaldırma ve taze tarama başarılıdır; eski satırı tutmak yerine güvenlik
                // duvarı uyarısını gösterirken doğrulanmış cache'i yeniden yükle.
                if (
                    error.code === 'FIREWALL_SYNC_FAILED' ||
                    error.code === 'MAIL_FILTER_SYNC_FAILED' ||
                    error.code === 'SERVICE_UNINSTALL_PARTIAL'
                ) {
                    closeUninstallDialog();
                    showToast('error', apiErrorText(error, t, 'services.actionFailed'));
                    if (!(await loadServices())) markStateUnverified();
                    return;
                }
                setUninstallError(error);
                return;
            }
            closeUninstallDialog();
            showToast(
                'success',
                nextCleanupAttempt !== null
                    ? t('services.webmailCleanupCompleted')
                    : t('services.uninstalled', { name: componentName(service, t) }),
            );
            if (!(await loadServices())) markStateUnverified();
        } catch {
            // A transport/read exception can happen after the server has already
            // removed Roundcube. Reset the stale retry handle before the
            // fail-closed lock disables this fieldset, otherwise the dialog's
            // Cancel and Retry controls become unreachable.
            closeUninstallDialog();
            markStateUnverified();
            showToast('error', t('services.actionFailed'));
        } finally {
            setBusy(null);
            setCleanupAttempt(null);
        }
    };

    // Inline actions are for `service` rows, whose one unit shares the
    // catalogue id. Per-version units (php8.3-fpm) go through
    // handleInstanceAction below — the "default"-sentinel unit guessing that
    // used to live here is gone with B3b.
    // Satır içi eylemler `service` satırları içindir; onların tek unit'i
    // katalog id'siyle aynıdır. Sürüm başına unit'ler (php8.3-fpm) aşağıdaki
    // handleInstanceAction'dan geçer — eskiden burada yaşayan "default"
    // sentinel'li unit tahmini B3b ile gitti.
    const handleAction = async (service: ManagedService, action: 'start' | 'stop' | 'restart') => {
        if (stateUnverified) {
            showToast('error', t('services.stateUnverifiedHint'));
            return;
        }
        setActionOutcome(null);
        setBusy(service.id);
        try {
            // Target the unit the SCAN found, not the catalogue id. They differ
            // exactly where this broke live: BIND's id is "bind" but its unit is
            // "named", so every stop/restart called a unit that does not exist
            // and the row just sat there (operator, 24 Jul).
            // Hedef, taramanın bulduğu unit'tir; katalog id'si değil. İkisi tam
            // da bunun canlıda kırıldığı yerde ayrışır: BIND'in id'si "bind",
            // unit'i "named"; bu yüzden her durdur/yeniden başlat var olmayan bir
            // unit'i çağırıyor ve satır olduğu yerde kalıyordu (operatör, 24 Tem).
            const res = await fetch('/api/v1/service/action', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name: service.unit || service.id, action }),
            });
            if (!res.ok) {
                // Say WHY. A generic "action failed" toast is why a real error
                // read as "nothing happened".
                // NEDENİNİ söyle. Genel "işlem başarısız" balonu, gerçek bir
                // hatanın "hiçbir şey olmadı" diye okunmasının sebebiydi.
                const error = await readApiError(res);
                if (error.code === 'SERVICE_STATE_REFRESH_FAILED') {
                    handleStateRefreshFailure(error);
                    return;
                }
                if (isServiceActionOutcome(error)) {
                    // The action may have changed the service, so the list is
                    // read again under the outcome.
                    // İşlem hizmeti değiştirmiş olabilir; liste sonucun
                    // altında yeniden okunur.
                    setActionOutcome(error);
                    if (!(await loadServices())) markStateUnverified();
                    return;
                }
                showToast('error', apiErrorText(error, t, 'services.actionFailed'));
                return;
            }
            // A success can leave something to know (a Stop that left the
            // unit marked as failed): it is said under the same notice.
            // Başarı, bilinmesi gereken bir şey bırakabilir.
            // An answer that cannot be read ends in the handler below: the
            // state is not verified, and nothing is assumed about it.
            const note = readServiceActionNote(await res.json());
            if (note) setActionOutcome(note);
            if (!(await loadServices())) markStateUnverified();
        } catch {
            // No answer arrived: the action may have been carried out. That is
            // not a failure; the notice above the list says the state is not
            // verified and offers the rescan.
            // Yanıt gelmedi: işlem yapılmış olabilir. Bu bir hata değildir;
            // listenin üstündeki bildirim durumun doğrulanmadığını söyler.
            markStateUnverified();
            showToast('warning', t('services.stateUnverifiedHint'));
        } finally {
            setBusy(null);
        }
    };

    // Start/stop ONE version of a runtime by its own unit. The endpoint
    // rescans server-side, so the follow-up load returns fresh per-instance
    // status.
    // Bir runtime'ın TEK sürümünü kendi unit'iyle başlat/durdur. Uç nokta
    // sunucu tarafında yeniden tarar; ardından gelen yükleme kopya-başına
    // taze durumu getirir.
    const handleInstanceAction = async (serviceId: string, unit: string, action: 'start' | 'stop' | 'restart') => {
        if (stateUnverified) {
            showToast('error', t('services.stateUnverifiedHint'));
            return;
        }
        setActionOutcome(null);
        setBusy(`${serviceId}:${unit}`);
        try {
            const res = await fetch('/api/v1/service/action', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name: unit, action }),
            });
            if (!res.ok) {
                const error = await readApiError(res);
                if (error.code === 'SERVICE_STATE_REFRESH_FAILED') {
                    handleStateRefreshFailure(error);
                    return;
                }
                if (isServiceActionOutcome(error)) {
                    // The action may have changed the service, so the list is
                    // read again under the outcome.
                    // İşlem hizmeti değiştirmiş olabilir; liste sonucun
                    // altında yeniden okunur.
                    setActionOutcome(error);
                    if (!(await loadServices())) markStateUnverified();
                    return;
                }
                showToast('error', apiErrorText(error, t, 'services.actionFailed'));
                return;
            }
            // A success can leave something to know (a Stop that left the
            // unit marked as failed): it is said under the same notice.
            // Başarı, bilinmesi gereken bir şey bırakabilir.
            // An answer that cannot be read ends in the handler below: the
            // state is not verified, and nothing is assumed about it.
            const note = readServiceActionNote(await res.json());
            if (note) setActionOutcome(note);
            if (!(await loadServices())) markStateUnverified();
        } catch {
            // No answer arrived: the action may have been carried out. That is
            // not a failure; the notice above the list says the state is not
            // verified and offers the rescan.
            // Yanıt gelmedi: işlem yapılmış olabilir. Bu bir hata değildir;
            // listenin üstündeki bildirim durumun doğrulanmadığını söyler.
            markStateUnverified();
            showToast('warning', t('services.stateUnverifiedHint'));
        } finally {
            setBusy(null);
        }
    };

    const isRunning = (s: ManagedService) => isServiceRunning(s.status);
    // Installed, with a unit of its own, and not running: the row's one red.
    // Kurulu, kendi unit'i var ve çalışmıyor: satırın tek kırmızısı.
    const rowStopped = (s: ManagedService) => s.is_installed === true && s.kind !== 'tool' && s.status !== 'installed' && !isRunning(s);

    // A requirement token is either a ROLE (seat name: any member satisfies
    // it — "smtp-server" after the operator's "maybe I'll install a different
    // SMTP server") or a component id. Roles localize, ids resolve to their
    // catalog display name; the raw token never reaches the screen.
    // Gereksinim belirteci ya bir ROLdür (koltuk adı: herhangi bir üye tatmin
    // eder — operatörün "belki başka bir SMTP sunucusu kurarım"ı sonrası
    // "smtp-server") ya da bileşen id'sidir. Roller yerelleşir, id'ler katalog
    // görünen adına çözülür; ham belirteç ekrana hiç çıkmaz.
    const reqLabel = (token: string) => {
        if (REQ_ROLE_KEYS[token]) return t(REQ_ROLE_KEYS[token] as Parameters<typeof t>[0]);
        return services.find((x) => x.id === token)?.name ?? token;
    };
    const reqNames = (tokens?: string[]) => (tokens ?? []).map(reqLabel).join(', ');
    const pageControlsBusy = scanning || busy !== null;
    const readinessMessage = hostMutationReadiness === null
        ? t('services.mutationReadiness.checking')
        : hostMutationReadiness.ready
            ? ''
            : t(
                `services.mutationReadiness.${hostMutationReadiness.reason ?? 'state_unverified'}` as Parameters<typeof t>[0],
            );
    const mutationControlsDisabled =
        pageControlsBusy
        || stateUnverified
        || hostMutationReadiness?.ready !== true;
    const openProfilePlan = (profile: ManagedMailProfile) => {
        if (stateUnverified || !profile.available) return;
        setProfileTarget(profile);
    };
    // Adds the profile to an extension of the completed setup and opens the
    // wizard on its access step; the owner reviews and starts it there. Only
    // the setup record changes here. A lost answer is settled by reading the
    // setup again, never by sending the request twice.
    // Profili kurulum genisletmesine ekler; inceleme ve baslatma kurulumdadir.
    const addProfileThroughSetup = async (chosenHostname: string): Promise<TranslationKey | null> => {
        const profile = profileTarget;
        if (!profile || !setupSnapshot || !serverSetup) return 'services.mailProfiles.setupRoute.failed';
        // code: the server's typed refusal; 'unanswered' when no answer came.
        let code = 'unanswered';
        let next: ServerSetupSnapshot | null = null;
        try {
            const response = await fetch('/api/v1/setup/extend', {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ revision: setupSnapshot.revision, action: 'begin', mail_profile: profile.id, ...(chosenHostname ? { mail_hostname: chosenHostname } : {}) }),
            });
            if (!response.ok) {
                code = (await readApiError(response)).code || 'refused';
            } else {
                next = decodeServerSetup(await response.json());
                code = '';
            }
        } catch { code = 'unanswered'; next = null; }
        if (!next?.extension) {
            let latest: ServerSetupSnapshot | null = null;
            try { latest = await serverSetup.reload(); } catch { latest = null; }
            if (code === 'server_setup_busy') return 'services.mailProfiles.setupRoute.busy';
            if (code === 'mail_profile_server_hostname_invalid' || code === 'mail_profile_server_hostname_required') return 'services.mailProfiles.hostname.invalid';
            // Without a confirmed answer the profile is not known to be in the
            // extension; the owner sees the setup as it is now and can retry.
            if (!latest?.extension || code !== 'unanswered') return 'services.mailProfiles.setupRoute.failed';
            next = latest;
        }
        serverSetup.accept(next);
        try {
            sessionStorage.setItem(setupEditorKey(user.username), JSON.stringify({ version: 1, revision: next.revision, step: 'access', draft: next.draft }));
        } catch { /* the wizard then opens on its first step */ }
        setProfileTarget(null);
        navigate('/setup');
        return null;
    };
    const installProfile = async (chosenHostname: string) => {
        const profile = profileTarget;
        if (!profile || stateUnverified || !profile.available) return;
        setProfileTarget(null);
        await startInstall({
            serviceId: profile.id,
            name: mailProfileName(profile, t),
            operationKind: 'mail_profile_install',
            ...(chosenHostname ? { mailHostname: chosenHostname } : {}),
        });
    };

    // Ready bundles (alpha.94, D-034). A mail bundle opens the mail install
    // dialogue of its profile, unchanged; a database or web bundle asks once
    // and then installs its missing components in order, one component
    // install at a time (alpha.101: it has no Verify; the page's Rescan reads
    // the components again).
    // Hazır paketler: posta paketi kendi profilinin penceresini açar; veritabanı
    // ve web paketi bir kez sorar ve eksik bileşenleri sırayla kurar.
    // Before every step the page's change gate is read again: an unverified
    // page state, or a host that is not ready for changes (read afresh, not
    // the last poll), stops the sequence with Continue.
    // Her adımdan önce sayfanın değişiklik kapısı yeniden okunur.
    // The gate runs inside the effect of the step that just ended, with the
    // closure of an earlier render: it reads the flag through the ref, which
    // is set synchronously, and again after the readiness read.
    // Kapı bayrağı ref üzerinden okur; ref eşzamanlı kurulur.
    const bundleGate = async (): Promise<string | null> => {
        if (stateUnverifiedRef.current) return t('services.stateUnverifiedHint');
        if (!(await loadHostMutationReadiness())) return t('services.bundles.gatedReadiness');
        if (stateUnverifiedRef.current) return t('services.stateUnverifiedHint');
        return null;
    };
    const installedNow = () => new Set(services.filter((service) => service.is_installed === true).map((service) => service.id));
    const bundleSequence = useBundleSequence(componentOperation, installedNow, (bundleId, componentId, index, total) => {
        const bundle = bundles.find((candidate) => candidate.id === bundleId);
        const component = componentName(services.find((service) => service.id === componentId) ?? componentId, t);
        const nameOf = (id: string) => componentName(services.find((service) => service.id === id) ?? id, t);
        return t('services.bundles.operationLabel', {
            component,
            bundle: bundle ? bundleName(bundle, t, nameOf) : component,
            n: index + 1,
            total,
        });
    }, bundleGate);
    // A mail bundle cannot start before the server's DNS identity is ready;
    // the Panel blocks it too, and the page does not offer it (D-021).
    // Repair opens the dialogue in its repair form whatever the profile's
    // state: it completes what is missing and proves the setup.
    // Posta paketi DNS kimliği hazır olmadan başlayamaz; Onar diyaloğu onarım
    // biçiminde açar.
    const openMailBundle = (bundle: ReadyBundle) => {
        if (!dnsIdentityReady) return;
        const profile = profiles.find((candidate) => candidate.id === bundle.profile_id);
        if (!profile) return;
        setProfileRepair(bundle.action === 'repair');
        setProfileFreshInstall(bundle.action !== 'repair' && bundleChipState(bundle, bundles) === 'available');
        openProfilePlan(profile);
    };
    const openComponentBundle = (bundle: ReadyBundle) => {
        if (mutationControlsDisabled) {
            showToast('warning', stateUnverified ? t('services.stateUnverifiedHint') : readinessMessage);
            return;
        }
        setBundleTarget(bundle);
    };
    const confirmComponentBundle = (choices: BundleChoices) => {
        const bundle = bundleTarget;
        setBundleTarget(null);
        if (!bundle || mutationControlsDisabled) return;
        bundleSequence.begin(bundle, choices);
    };
    // The mail strip's one-line notices, in place of the boxes that stood
    // above the mail stack cards: why mail cannot be installed yet, who acts
    // and where (owner decision, 2026-10-17).
    // Posta şeridinin tek satırlık notları: posta neden henüz kurulamaz, kim
    // ve nerede işlem yapar.
    const mailSecondary = !dnsIdentityReady && mailDNSGuidance !== null;
    const setupReady = setupSnapshot?.status === 'ready' || setupSnapshot?.extension === true;
    const mailNotice = mailSecondary && mailDNSGuidance ? (
        <div role='status' className='min-w-0 basis-full text-sm text-fg' data-mail-dns-secondary=''>
            <p className='flex flex-wrap items-baseline gap-x-2 gap-y-1'>
                <span>{t('services.mailProfiles.secondary.short')}</span>
                <span className='inline-flex flex-wrap gap-x-2 break-all rounded-md border border-border bg-surface px-2 py-0.5 text-xs' data-mail-dns-record=''>
                    <span className={mailDNSGuidance.recordName ? 'font-mono' : 'text-fg-muted'}>{mailDNSGuidance.recordName || t('services.mailProfiles.secondary.nameUnknown')}</span>
                    <span className='font-mono'>A</span>
                    <span className={mailDNSGuidance.recordValue ? 'font-mono' : 'text-fg-muted'}>{mailDNSGuidance.recordValue || t('services.mailProfiles.secondary.valueUnknown')}</span>
                </span>
                <span className='text-fg-subtle' aria-hidden='true'>·</span>
                <button type='button' onClick={() => navigate('/setup')} className='font-semibold text-primary underline decoration-1 underline-offset-[3px] hover:decoration-2 max-sm:min-h-11'>
                    {t(setupReady ? 'services.mailProfiles.secondary.extend' : 'services.mailProfiles.secondary.openSetup')}
                </button>
            </p>
            <details className='mt-1 text-xs text-fg-muted'>
                <summary className='cursor-pointer font-medium text-fg-muted max-sm:min-h-11'>{t('services.bundles.mailWhy')}</summary>
                <p className='mt-1 max-w-[75ch] leading-5'>
                    {mailDNSGuidance.primaryNameserver
                        ? t('services.mailProfiles.secondary.body', { primary: mailDNSGuidance.primaryIP ? `${mailDNSGuidance.primaryNameserver} (${mailDNSGuidance.primaryIP})` : mailDNSGuidance.primaryNameserver })
                        : t('services.mailProfiles.secondary.bodyUnnamed')}
                </p>
                <p className='mt-1 max-w-[75ch] leading-5' data-mail-dns-channel={mailDNSGuidance.peerRecordChannel ? 'yes' : 'no'}>
                    {t(mailDNSGuidance.peerRecordChannel ? 'services.mailProfiles.secondary.channel' : 'services.mailProfiles.secondary.noChannel')}
                </p>
                <p className='mt-1 max-w-[75ch] leading-5'>{t('services.mailProfiles.secondary.next')}</p>
            </details>
        </div>
    ) : !dnsIdentityReady && bundles.some((bundle) => bundle.category === 'email' && bundle.status !== 'unknown') ? (
        <p role='status' className='flex min-w-0 basis-full flex-wrap items-baseline gap-x-2 text-sm text-fg' data-mail-dns-required=''>
            <span>{t('services.mailProfiles.dnsRequiredShort')}</span>
            <span className='text-fg-subtle max-sm:hidden' aria-hidden='true'>·</span>
            <button type='button' onClick={() => navigate('/dns-infrastructure')} className='font-semibold text-primary underline decoration-1 underline-offset-[3px] hover:decoration-2 max-sm:min-h-11'>
                {t('services.mailProfiles.configureDNS')}
            </button>
        </p>
    ) : undefined;

    const requestComponentAction = (action: PendingComponentAction) => {
        if (mutationControlsDisabled) {
            showToast('warning', stateUnverified ? t('services.stateUnverifiedHint') : readinessMessage);
            return;
        }
        setPendingAction(action);
    };

    const confirmComponentAction = async () => {
        const action = pendingAction;
        if (!action || hostMutationReadiness?.ready !== true || stateUnverified) return;
        setPendingAction(null);
        switch (action.kind) {
        case 'service':
            await handleAction(action.service, action.action);
            break;
        case 'instance':
            await handleInstanceAction(action.service.id, action.unit, action.action);
            break;
        case 'repair':
            await startInstall({
                serviceId: action.service.id,
                name: componentName(action.service, t),
                ...(action.service.repair_package ? { package: action.service.repair_package } : {}),
            });
            break;
        case 'install-package':
            await startInstall({
                serviceId: action.service.id,
                package: action.pkg,
                name: `${componentName(action.service, t)} ${action.version}`,
            });
            break;
        case 'install-node':
            await startInstall({
                serviceId: action.service.id,
                version: action.version,
                name: `Node ${action.version}`,
            });
            break;
        }
        void loadHostMutationReadiness();
    };

    return (
        <div className="p-6 md:p-8">
            <PageHeader
                title={t('nav.services')}
                subtitle={t('services.subtitle')}
                breadcrumb={[t('common.home'), t('nav.services')]}
            />

            <FirewallBar
                place="components"
                readiness={hostMutationReadiness}
                readinessMessage={readinessMessage}
                onRefreshReadiness={loadHostMutationReadiness}
                refreshKey={scanRound}
            />

            {hostMutationReadiness?.ready === false && (
                <section role='status' className='mb-4 rounded-xl border border-warning-mark/60 bg-warning-mark/20 px-4 py-3 text-sm text-fg'>
                    <span className='font-semibold'>{t('services.mutationReadiness.title')}</span>{' '}
                    {readinessMessage}{' '}
                    {/* The readiness poll above lifts this by itself (owner rule 2026-10-14). */}
                    {t('services.mutationReadiness.autoResume')}
                </section>
            )}

            <div className="mb-4 flex flex-wrap items-center gap-3">
                <button
                    onClick={scan}
                    disabled={pageControlsBusy}
                    className="inline-flex items-center gap-2 rounded-lg border border-border-strong bg-surface px-3 py-1.5 text-sm font-medium text-fg transition-colors hover:bg-surface-2 disabled:opacity-50 max-sm:min-h-11"
                >
                    <RefreshCw className={`h-4 w-4 ${scanning ? 'animate-spin' : ''}`} />
                    {scanning ? t('services.scanning') : scannedAt ? t('services.rescan') : t('services.scanNow')}
                </button>
                {/* Two different sentences the operator must be able to
                    tell apart: "I am looking at this server now" and "I am
                    showing you what was last seen". While a check runs, the
                    page says the first — and, when a cached snapshot is still
                    on screen underneath, it says the second in the same
                    breath, so the rows below are never mistaken for what the
                    running check is about to find.
                    Operatörün ayırt edebilmesi gereken iki ayrı cümle: "şu an
                    bu sunucuya bakıyorum" ve "sana en son görüleni
                    gösteriyorum". Kontrol koşarken sayfa birincisini söyler ve
                    altta hâlâ önbellek görüntüsü duruyorsa ikincisini de aynı
                    nefeste söyler; böylece aşağıdaki satırlar, koşan kontrolün
                    birazdan bulacağı şeyle karıştırılmaz. */}
                <span role="status" aria-live="polite" className="flex flex-wrap items-center gap-x-2 text-xs text-fg-subtle">
                    {scanning ? (
                        <>
                            <span className="font-medium text-fg-muted">{t('services.checkingNow')}</span>
                            {scannedAt && <span>{t('services.showingLastSeen')}</span>}
                        </>
                    ) : scannedAt ? (
                        t('services.lastScan', { time: new Date(scannedAt).toLocaleString() })
                    ) : (
                        t('services.neverScannedShort')
                    )}
                </span>
                {scannedAt && services.length > 0 && (
                    // justify-end on a row that cannot fit pushes the overflow
                    // off the LEFT edge, where nothing can reach it: at 390px
                    // the two view buttons were clipped by the viewport rather
                    // than wrapped (register R-047). The row is end-aligned only
                    // once there is room to end-align it; below that it starts
                    // at the left margin like everything else on the page, and
                    // the view cluster wraps instead of overflowing.
                    // justify-end, sığmayan bir satırda taşmayı SOL kenardan
                    // dışarı iter; orada ona kimse ulaşamaz: 390px'te iki
                    // görünüm düğmesi sarılmak yerine görünür alan tarafından
                    // kırpılıyordu (defter R-047). Satır ancak sağa yaslanacak
                    // yer varken sağa yaslanır; altında, sayfadaki her şey gibi
                    // sol kenardan başlar ve görünüm kümesi taşmak yerine alt
                    // satıra iner.
                    <div className="ml-auto flex w-full flex-wrap items-center justify-start gap-x-5 gap-y-2 sm:w-auto sm:justify-end">
                        <div className="w-full sm:w-56">
                            <SearchInput value={query} onChange={setQuery} placeholder={t('services.search')} />
                        </div>
                        {/* View controls kept as one tight cluster, clearly
                            separated from the search by the larger gap-x-5. */}
                        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                            {/* Two named views instead of one checkbox: each
                                says what it shows and how many, so "where do I
                                install from?" is answered by reading, not by
                                discovering a filter.
                                Tek onay kutusu yerine adı yazan iki görünüm:
                                her biri neyi kaç kalemle gösterdiğini söyler;
                                "nereden kurarım?" sorusu süzgeç keşfederek
                                değil okuyarak cevaplanır. */}
                            <div className="inline-flex overflow-hidden rounded-lg border border-border-strong">
                                <button
                                    onClick={() => setView(true)}
                                    className={`whitespace-nowrap px-3 py-1.5 text-xs font-medium transition-colors max-sm:min-h-11 ${
                                        hideNotInstalled ? 'bg-primary text-primary-fg' : 'bg-surface text-fg-muted hover:bg-surface-2'
                                    }`}
                                >
                                    {t('services.viewInstalled', { n: services.filter((s) => s.is_installed).length })}
                                </button>
                                <button
                                    onClick={() => setView(false)}
                                    className={`whitespace-nowrap border-l border-border-strong px-3 py-1.5 text-xs font-medium transition-colors max-sm:min-h-11 ${
                                        hideNotInstalled ? 'bg-surface text-fg-muted hover:bg-surface-2' : 'bg-primary text-primary-fg'
                                    }`}
                                >
                                    {t('services.viewCatalog', { n: services.length })}
                                </button>
                            </div>
                            <div className="flex gap-1 text-xs">
                                <button onClick={expandAll} className="whitespace-nowrap rounded-md px-2 py-1 max-sm:min-h-11 text-fg-muted hover:bg-surface-2 hover:text-fg">
                                    {t('services.expandAll')}
                                </button>
                                <button onClick={collapseAll} className="whitespace-nowrap rounded-md px-2 py-1 max-sm:min-h-11 text-fg-muted hover:bg-surface-2 hover:text-fg">
                                    {t('services.collapseAll')}
                                </button>
                            </div>
                        </div>
                    </div>
                )}
            </div>

            <ServiceActionNotice outcome={actionOutcome} onClose={() => setActionOutcome(null)} className="mb-4" />

            {/* Two different situations, two sentences (D-024): an action was
                sent and its result is unknown, or nothing was sent and the
                state could not be read (set14b O2: with the Agent stopped the
                page said an action "may have completed" when none had run).
                İki ayrı durum, iki cümle: işlem gönderildi ve sonucu bilinmiyor,
                ya da hiçbir şey gönderilmedi ve durum okunamadı. */}
            {stateUnverified && verification.cause.kind === 'action' && (
                <div role="alert" className="mb-4 flex items-start gap-3 rounded-xl border border-warning-mark/60 bg-warning-mark/10 p-4">
                    <ShieldOff className="mt-0.5 h-5 w-5 shrink-0 text-warning" />
                    <div>
                        <div className="text-sm font-semibold text-fg">{t('services.stateUnverifiedTitle')}</div>
                        <p className="text-sm text-fg-muted">{t('services.stateUnverifiedHint')}</p>
                    </div>
                </div>
            )}
            {stateUnverified && verification.cause.kind === 'read' && (
                <div role="alert" className="mb-4 flex flex-wrap items-start gap-3 rounded-xl border border-warning-mark/60 bg-warning-mark/10 p-4">
                    <ShieldOff className="mt-0.5 h-5 w-5 shrink-0 text-warning" />
                    <div className="min-w-0 grow basis-56">
                        <div className="text-balance text-sm font-semibold text-fg">{t('services.stateUnreadTitle')}</div>
                        <p className="mt-0.5 max-w-[75ch] text-sm text-fg-muted">{t('services.stateUnreadHint', {
                            reason: verification.cause.error ? apiErrorText(verification.cause.error, t, 'services.scanFailed') : t('services.scanFailed'),
                        })}</p>
                    </div>
                    <div className="ml-auto shrink-0">
                        <Button variant="secondary" icon={RefreshCw} onClick={scan} disabled={pageControlsBusy}>
                            {scanning ? t('services.scanning') : t('services.rescan')}
                        </Button>
                    </div>
                </div>
            )}

            {/* The honest first state of a server nobody has scanned: the
                catalogue below is real, its installed/absent column is not
                known yet, and the one thing that resolves that is one click
                away. Neutral, not warning-coloured — semantic colour is
                reserved for something actually being wrong, and a host that
                has simply not been looked at yet is the normal beginning.
                Hiç taranmamış bir sunucunun dürüst ilk durumu: aşağıdaki
                katalog gerçektir, kurulu/yok sütunu ise henüz bilinmiyor ve
                bunu çözecek tek şey bir tık ötede. Nötr, uyarı renginde değil
                — anlam taşıyan renk gerçekten bozuk olan için ayrılmıştır. */}
            {!loading && hostNeverChecked && (
                <section role="status" className="mb-4 flex flex-wrap items-start gap-3 rounded-xl border border-border bg-surface p-4">
                    <ScanSearch className="mt-0.5 h-5 w-5 shrink-0 text-fg-muted" />
                    {/* The same rule as the setup journey's rows: `flex-1`
                        alone bases this block at zero width, so at 390px the
                        button kept the row on one line and squeezed the text
                        to 122px - three lines of a title that fits on one. A
                        real basis wraps the button onto its own line instead.
                        Kurulum yolculuğu satırlarıyla aynı kural: tek başına
                        `flex-1` bu bloğu sıfır genişlikte tabanlar; 390px'te
                        düğme satırı tek satırda tutup metni 122px'e sıkıştırdı
                        - bir satıra sığan başlık üç satır oldu. */}
                    <div className="min-w-0 grow basis-56">
                        {/* text-balance: at 390px this title left "yet" alone
                            on its own line. Balanced wrapping splits it evenly
                            instead of stranding the last word.
                            text-balance: 390px'te bu başlık son sözcüğü tek
                            başına alt satıra bırakıyordu. */}
                        <div className="text-balance text-sm font-semibold text-fg">{t('services.notCheckedTitle')}</div>
                        <p className="mt-0.5 text-sm text-fg-muted">{t('services.notCheckedHint')}</p>
                        {/* The fallback state, named. Reaching "not checked
                            yet" because the automatic check FAILED is not the
                            same as reaching it because no check has run, and
                            only one of the two is fixed by pressing the button
                            again. This is a real failure, so it may carry the
                            warning colour the calm card around it may not.
                            Geri düşülen durum, adıyla. "Henüz bakılmadı"ya
                            otomatik kontrol BAŞARISIZ olduğu için gelmek, hiç
                            kontrol koşmadığı için gelmekle aynı şey değildir.
                            Bu gerçek bir hatadır; çevresindeki sakin kartın
                            taşıyamayacağı uyarı rengini taşıyabilir. */}
                        {autoCheckFailed && (
                            <p className="mt-1.5 text-sm font-medium text-warning">{t('services.autoCheckFailed')}</p>
                        )}
                    </div>
                    <div className="ml-auto shrink-0">
                        <Button variant="primary" icon={ScanSearch} onClick={scan} disabled={pageControlsBusy}>
                            {scanning ? t('services.scanning') : t('services.scanNow')}
                        </Button>
                    </div>
                </section>
            )}


            {loading ? (
                <div className="flex flex-col items-center justify-center gap-3 py-16 text-sm text-fg-muted">
                    <Spinner />
                    {scanning && <span>{t('services.autoScanning')}</span>}
                </div>
            ) : !scannedAt && services.length === 0 ? (
                <EmptyState
                    icon={ScanSearch}
                    title={t('services.neverScanned')}
                    hint={t('services.neverScannedHint')}
                />
            ) : (
                <>
                <p className="mb-2 text-xs text-fg-subtle">
                    {filtered.length === services.length
                        ? (services.length === 1 ? t('common.itemsTotalOne') : t('common.itemsTotal', { n: services.length }))
                        : t('services.matchCount', { shown: filtered.length, total: services.length })}
                </p>
                {filtered.length === 0 && (
                    <div className="mb-4 rounded-xl border border-border bg-surface p-8 text-center text-sm text-fg-muted">
                        {/* A fresh server's installed view is legitimately
                            empty — that is a starting point, not a failed
                            search, so it offers the next step instead of
                            reporting nothing.
                            Taze sunucunun kurulu görünümü meşru olarak boştur —
                            bu başarısız arama değil başlangıç noktasıdır; hiçlik
                            bildirmek yerine sonraki adımı sunar. */}
                        {hideNotInstalled && q === '' ? (
                            <div className="flex flex-col items-center gap-3">
                                <span>{t('services.noneInstalledYet')}</span>
                                <Button variant="primary" icon={DownloadCloud} onClick={() => setView(false)}>
                                    {t('services.browseCatalog')}
                                </Button>
                            </div>
                        ) : (
                            t('services.noMatches')
                        )}
                    </div>
                )}
                {/* With nothing installed the installed view still draws each
                    category's ready bundles strip: the strip is the offer
                    (alpha.94 review).
                    Hiçbir şey kurulu değilken de kurulu görünüm şeritleri çizer. */}
                {(filtered.length > 0 || (q === '' && bundles.length > 0)) && (
                <div id='bundles' tabIndex={-1} className="space-y-4 scroll-mt-24 outline-none">
                    {/* A component whose category is not in categoryOrder used to
                        render NOWHERE: the map only walks known categories and
                        there was no leftover bucket — so a newly added component
                        installed fine and was invisible on this page, while still
                        counting in the "Catalog (n)" button. Adding the unknown
                        categories to the walk means a new component always shows
                        up somewhere, which is the whole promise of this page.
                        Kategorisi categoryOrder'da olmayan bir bileşen HİÇBİR
                        YERDE çizilmiyordu: map yalnız bilinen kategorileri
                        dolaşıyor ve artık kova yoktu — yani yeni eklenen bileşen
                        sorunsuz kuruluyor ama bu sayfada görünmüyordu, üstelik
                        "Katalog (n)" sayacında görünmeye devam ediyordu.
                        Bilinmeyen kategorileri de dolaşıma katmak, yeni bir
                        bileşenin her zaman bir yerde görünmesi demektir; bu
                        sayfanın bütün vaadi de budur. */}
                    {[...categoryOrder, ...unknownCategories(filtered)].map(({ id: cat, labelKey, icon: CatIcon, tint }) => {
                        const group = filtered.filter((s) => s.category === cat);
                        // A category whose rows the installed view hides
                        // still shows its ready bundles strip (alpha.94).
                        // Kurulu görünümün satırlarını gizlediği bölüm de
                        // hazır paket şeridini gösterir.
                        const hasBundles = q === '' && bundles.some((bundle) => bundle.category === cat);
                        if (group.length === 0 && !hasBundles) return null;
                        const counted = group.length > 0 ? group : services.filter((s) => s.category === cat);
                        const installedCount = counted.filter((s) => s.is_installed).length;
                        // "0/6" would be a count of a census nobody took.
                        // A group nothing is known about shows a dash.
                        // "0/6", kimsenin yapmadığı bir sayımın sonucu olurdu.
                        const groupChecked = counted.some((s) => !notChecked(s));
                        const isOpen = q !== '' || !collapsed.has(cat);
                        return (
                            <section key={cat} className="overflow-hidden rounded-xl border border-border-strong bg-surface">
                                <button
                                    type="button"
                                    onClick={() => toggleGroup(cat)}
                                    className="flex w-full items-center gap-3 px-4 py-3.5 text-left transition-colors hover:bg-surface-2/60"
                                >
                                    <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${tint}`}>
                                        <CatIcon className="h-5 w-5" />
                                    </span>
                                    <span className="text-lg font-semibold text-fg">{t(labelKey as Parameters<typeof t>[0])}</span>
                                    <span className="text-sm text-fg-subtle tabular-nums">
                                        {groupChecked ? `${installedCount}/${counted.length}` : '—'}
                                    </span>
                                    <span className="ml-auto text-fg-subtle">
                                        {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                                    </span>
                                </button>
                                {isOpen && hasBundles && (
                                    <ReadyBundleStrip
                                        category={cat}
                                        bundles={bundles}
                                        services={services}
                                        profiles={profiles}
                                        run={bundleSequence.run}
                                        disabled={mutationControlsDisabled || componentOperation.locked || (cat === 'email' && !dnsIdentityReady)}
                                        scanning={scanning}
                                        notice={cat === 'email' ? mailNotice : undefined}
                                        onMail={openMailBundle}
                                        onComponents={openComponentBundle}
                                    />
                                )}
                                {isOpen && group.length > 0 && (
                                    <ul className="border-t border-border">
                                        {group.map((s) => {
                                            const running = isRunning(s);
                                            return (
                                                <li key={s.id} className={`relative flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-border px-4 py-3.5 last:border-0 hover:bg-surface-2/40 md:grid ${versionColumn ? 'md:grid-cols-[minmax(0,1fr)_7.5rem_8rem_17rem]' : 'md:grid-cols-[minmax(0,1fr)_8rem_17rem]'}`}>
                                                    {/* Selective dimming, not block opacity: the name stays
                                                        readable in the light theme; state is carried by the
                                                        muted tone + status text + Install button.
                                                        / Blok saydamlık değil seçici soldurma: ad açık temada
                                                        da okunur kalır; durumu soluk ton + durum yazısı +
                                                        Kur düğmesi anlatır. */}
                                                    <div className="flex min-w-0 flex-1 basis-52 items-center gap-3">
                                                        {/* One line icon per component, from one map (alpha.94);
                                                            the Panel's emoji is not drawn. A row that is not
                                                            installed keeps the muted tint.
                                                            Bileşen başına tek çizgi ikon, tek eşlemden. */}
                                                        <RowIcon id={s.id} category={s.category} installed={s.is_installed === true} />
                                                        <div className="min-w-0">
                                                            {/* Every row is a door to its own page. Manage used
                                                                to be the only one, and it is drawn for an
                                                                INSTALLED non-tool alone — so on a fresh host,
                                                                where nothing is installed and nothing has been
                                                                checked, the page that explains each component's
                                                                state was reachable only by typing its URL.
                                                                The row itself is now a real link: it opens with
                                                                a click, with the keyboard, and in a new tab on
                                                                middle-click, and its accessible name is the
                                                                component's. Manage stays the primary action
                                                                where it exists — this is the row, not a second
                                                                button competing with it.
                                                                Her satır kendi sayfasının kapısıdır. Eskiden tek
                                                                kapı Yönet'ti ve o yalnız KURULU, tool olmayan
                                                                bileşene çizilir — yani hiçbir şeyin kurulu
                                                                olmadığı, hiçbir şeye bakılmadığı taze makinede,
                                                                her bileşenin durumunu anlatan sayfaya ancak
                                                                adresi elle yazarak ulaşılabiliyordu. Artık
                                                                satırın kendisi gerçek bir bağlantıdır: tıkla,
                                                                klavyeyle ve orta tıkla yeni sekmede açılır;
                                                                erişilebilir adı bileşenin adıdır. Yönet, var
                                                                olduğu yerde birincil eylem olarak kalır. */}
                                                            <Link
                                                                to={`/services/${s.id}`}
                                                                className={`rounded-md text-base font-medium outline-none after:absolute after:inset-0 after:content-[''] focus-visible:ring-2 focus-visible:ring-primary/40 ${s.is_installed ? 'text-fg' : 'text-fg-muted'}`}
                                                            >
                                                                {componentName(s, t)}
                                                            </Link>
                                                            <div className="text-pretty text-xs text-fg-subtle">{componentDescription(s, t)}</div>
                                                            {/* What "Clean up leftovers" is about, said on the
                                                                row only when the Panel's own record shows a
                                                                Roundcube install or removal that did not finish
                                                                (or a failed webmail attempt). The scan cannot tell
                                                                whether files are left, so the line says "may";
                                                                without a record the row says nothing and the
                                                                cleanup stays on the component's page (alpha.95
                                                                review).
                                                                Yalnız Panel'in kaydı bitmeyen bir kurulum ya da
                                                                kaldırma gösterdiğinde; kayıt yoksa satır susar. */}
                                                            {s.id === 'roundcube' && s.is_installed === false && webmailCleanupOffered && (
                                                                <div className="text-pretty text-xs text-fg-muted" data-webmail-leftovers={webmailCleanupNote}>
                                                                    {t(webmailCleanupNote === 'removal' ? 'services.cleanupWebmailRowNote.removal' : 'services.cleanupWebmailRowNote.install')}
                                                                </div>
                                                            )}
                                                            {/* The version is said under the description, only
                                                                where the scan knows it. The scan reads the
                                                                versions of the multi-version runtimes alone
                                                                (PHP-FPM, Node.js); a column of dashes for every
                                                                other component said nothing (owner's screens,
                                                                2026-10-17), so the desktop column is gone until
                                                                the installed package versions are read (open
                                                                roadmap item).
                                                                Sürüm, yalnız tarama bildiğinde açıklamanın altında
                                                                söylenir; masaüstü sütunu kaldırıldı. */}
                                                            {/* alpha.97: with the desktop column drawn, this
                                                                line is the phone's (as before alpha.95);
                                                                without it, the only place a version is said.
                                                                Sütun varsa bu satır telefonundur. */}
                                                            <RowVersion service={s} className={versionColumn ? 'md:hidden' : ''} />
                                                            {/* alpha.104: a PHP tool whose PHP version is not there
                                                                says why and where it is fixed, under its name.
                                                                PHP'si olmayan araç nedenini ve çaresini söyler. */}
                                                            {componentStateWord(s) === 'notReady' && phpBindingOf(s) && (
                                                                <p className="mt-0.5 text-xs text-fg-muted" data-row-not-ready="">
                                                                    {t(phpNotReadyHintKey(phpBindingOf(s)!), { version: phpBindingOf(s)!.version, extensions: phpBindingOf(s)!.proof_extensions.join(', '), modules: phpBindingOf(s)!.proof_extensions.join(' ') })}
                                                                </p>
                                                            )}
                                                        </div>
                                                    </div>
                                                    {versionColumn && (
                                                        <div className="hidden min-w-0 md:block" data-version-column="">
                                                            <RowVersion service={s} column />
                                                        </div>
                                                    )}
                                                    <div className="w-32 shrink-0 md:w-auto">
                                                        {/* "Not checked yet" is information the operator
                                                            acts on, so it is fg-muted like a real status —
                                                            fg-subtle is for disabled and placeholder text
                                                            and must not carry meaning. Only the state we
                                                            actually verified as absent recedes.
                                                            "Henüz bakılmadı" operatörün üzerine hareket
                                                            ettiği bir bilgidir; bu yüzden gerçek bir durum
                                                            gibi fg-muted'tır. */}
                                                        <span className={`inline-flex items-center gap-1.5 text-sm ${s.is_installed === false ? 'text-fg-subtle' : rowStopped(s) || componentStateWord(s) === 'notReady' ? 'font-medium text-danger' : 'text-fg-muted'}`} data-row-state={componentStateWord(s)}>
                                                            {/* A tool has no daemon of ours; a runtime whose
                                                                copies have no units either (node — executed
                                                                only by per-site apps) reports status
                                                                "installed". For both, "installed" is the whole
                                                                truth. php-fpm HAS units and keeps
                                                                running/stopped: a dead php-fpm must still
                                                                raise the alarm (D-010/B3b).
                                                                Tool'un bize ait daemon'ı yok; kopyalarının
                                                                unit'i olmayan runtime da (node — onu yalnız
                                                                site başına uygulamalar çalıştırır) "installed"
                                                                durumu bildirir. İkisi için de "kurulu" tam
                                                                gerçektir. php-fpm'in unit'leri VARDIR ve
                                                                çalışıyor/durdu kalır: ölü php-fpm yine alarm
                                                                vermelidir (D-010/B3b). */}
                                                            {/* Four tones (alpha.94): running, stopped (the one
                                                                state that must stand out), not installed (a
                                                                hollow ring) and not checked.
                                                                Dört ton: çalışıyor, durdu, kurulu değil, bakılmadı. */}
                                                            {/* The installed bundle's dialogue says the same word
                                                                for each part (componentStateWord in
                                                                lib/serviceStatus.ts, alpha.101); the two are
                                                                held equal by ready-bundles-mounted.
                                                                Kurulu paket penceresi aynı sözü söyler. */}
                                                            <StatusDot tone={notChecked(s) ? 'unknown' : !s.is_installed ? 'idle' : componentStateWord(s) === 'notReady' ? 'danger' : (running || s.kind === 'tool' || s.status === 'installed') ? 'success' : 'danger'} />
                                                            {notChecked(s)
                                                                ? t('services.notChecked')
                                                                : !s.is_installed
                                                                ? t('services.notInstalled')
                                                                : componentStateWord(s) === 'notReady'
                                                                  ? t('services.phpNotReady')
                                                                  : s.kind === 'tool' || s.status === 'installed'
                                                                    ? t('services.installedLabel')
                                                                    : running
                                                                      ? t('services.running')
                                                                      : t('services.stopped')}
                                                        </span>

                                                    </div>
                                                    <fieldset
                                                        disabled={mutationControlsDisabled}
                                                        aria-busy={pageControlsBusy}
                                                        /* relative: the row link stretches a pseudo-element
                                                           across the whole row, so every real control has to
                                                           sit above it or it would be unclickable.
                                                           relative: satır bağlantısı sözde-elemanını bütün
                                                           satıra yayar; gerçek denetimler onun üstünde
                                                           durmalıdır, yoksa tıklanamazlar. */
                                                        className="relative ml-auto flex min-w-0 flex-wrap items-center justify-end gap-1 border-0 p-0 max-sm:ml-0 max-sm:w-full max-sm:justify-start"
                                                    >
                                                    {!s.is_installed ? (
                                                    <>
                                                    {/* An interrupted uninstall can leave Roundcube application
                                                        data after the package scan says it is not installed. Keep
                                                        cleanup reachable across reloads, independently of install
                                                        and repair availability. */}
                                                    {/* Nothing to offer about a component nobody has looked
                                        at: an Install button here would be a guess, and a
                                        "conflicts with" or "requires" badge would be a claim
                                        about an inventory that was never taken. The check
                                        itself is the notice above the list, and Rescan in the
                                        page header — one action, not thirty identical ones.
                                        Kimsenin bakmadığı bir bileşen hakkında sunulacak bir
                                        şey yok: buradaki Kur düğmesi tahmin, "çakışıyor" ya
                                        da "gerekli" rozeti ise hiç yapılmamış bir envanter
                                        hakkında iddia olurdu. */}
                                    {notChecked(s) ? null : (s.id === 'pdns' || s.id === 'bind') ? (
                                                            <button
                                                                type="button"
                                                                onClick={() => navigate('/dns-infrastructure')}
                                                                title={t('services.manageDNSEngineHint')}
                                                                className={rowButtonClass}
                                                            >
                                                                <Settings className="h-3.5 w-3.5" />
                                                                {t('services.manageDNSEngine')}
                                                            </button>
                                                        ) : s.conflict_with ? (
                                                            <span
                                                                title={t('services.conflictHint', { name: s.conflict_with })}
                                                                className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-surface-2 px-3 py-1.5 text-xs font-medium text-fg-subtle"
                                                            >
                                                                <span><KeepWords text={t('services.conflictWith', { name: s.conflict_with })} /></span>
                                                            </span>
                                                        ) : s.not_offered_kind === 'integration' && s.id === 'vsftpd' ? (
                                                            <button
                                                                type="button"
                                                                onClick={() => navigate('/domains')}
                                                                title={t('services.useBuiltInSFTPHint')}
                                                                className="inline-flex items-center gap-1.5 rounded-lg border border-primary/30 bg-primary/10 px-3 py-1.5 text-xs font-semibold text-primary transition-colors hover:bg-primary/15 disabled:cursor-not-allowed disabled:opacity-50"
                                                            >
                                                                {t('services.useBuiltInSFTP')}
                                                                <ChevronRight className="h-3.5 w-3.5" />
                                                            </button>
                                                        ) : s.not_offered ? (
                                                            /* Honest instead of a dead Install button that
                                                               fails late in the agent (spamassassin on Arch,
                                                               netdata on Debian). The full per-distro list
                                                               lives in docs/DISTRO-SUPPORT.
                                                               Agent'ta geç patlayan ölü bir Kur düğmesi yerine
                                                               dürüstlük (Arch'ta spamassassin, Debian'da
                                                               netdata). Dağıtım başına tam liste
                                                               docs/DISTRO-SUPPORT içinde. */
                                                            <span
                                                                title={s.not_offered_kind === 'integration'
                                                                    ? t('services.integrationPendingHint')
                                                                    : (s.not_offered_reason || t('services.notOfferedHint'))}
                                                                className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-surface-2 px-3 py-1.5 text-xs font-medium text-fg-subtle"
                                                            >
                                                                {s.not_offered_kind === 'integration'
                                                                    ? t('services.integrationPending')
                                                                    : t('services.notOffered')}
                                                            </span>
                                                        ) : s.requires_missing && s.requires_missing.length > 0 ? (
                                                            <span
                                                                title={t('services.requiresHint', { names: reqNames(s.requires_missing) })}
                                                                className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-surface-2 px-3 py-1.5 text-xs font-medium text-fg-subtle"
                                                            >
                                                                {/* A name such as PHP-FPM never breaks at its
                                                                    hyphen (owner's screens, 2026-10-17).
                                                                    PHP-FPM gibi bir ad kısa çizgisinden bölünmez. */}
                                                                <span><KeepWords text={t('services.requiresLabel', { names: reqNames(s.requires_missing) })} /></span>
                                                            </span>
                                                        ) : (
                                                        <button
                                                            /* A tarball runtime (node) is not an apt/pacman
                                                               package — Install opens the version drawer,
                                                               where versions come from upstream.
                                                               Tarball runtime (node) apt/pacman paketi değil —
                                                               Kur, sürüm çekmecesini açar; sürümler kaynaktan
                                                               gelir. */
                                                            onClick={() => (isTarballRuntime(s) ? toggleDrawer(s.id) : setInstallTarget(s))}
                                                            disabled={busy === s.id}
                                                            className={rowButtonClass}
                                                        >
                                                            <DownloadCloud className="h-3.5 w-3.5" />
                                                            {busy === s.id ? t('services.installing') : t('services.install')}
                                                        </button>
                                                        )
                                                    }
                                                    {/* A neutral secondary button (alpha.95): the red
                                                        stays on the confirmation, which lists what is
                                                        removed.
                                                        Nötr ikincil düğme; kırmızı, neyin silineceğini
                                                        sayan onaydadır. */}
                                                    {s.id === 'roundcube' && webmailCleanupOffered && (
                                                    <button
                                                        type={'button'}
                                                        onClick={() => openWebmailCleanupDialog(s)}
                                                        disabled={mutationControlsDisabled || busy === s.id}
                                                        title={t('services.cleanupWebmailHint')}
                                                        className={rowButtonClass}
                                                        data-webmail-cleanup=""
                                                    >
                                                        <Trash2 className={'h-3.5 w-3.5'} aria-hidden="true" />
                                                        {t('services.cleanupWebmailRow')}
                                                    </button>
                                                    )}
                                                    </>
                                                    ) : (s.id === 'pdns' || s.id === 'bind') ? (
                                                    <button
                                                        type="button"
                                                        onClick={() => navigate('/dns-infrastructure')}
                                                        title={t('services.manageDNSEngineHint')}
                                                        className={rowButtonClass}
                                                    >
                                                        <Settings className="h-3.5 w-3.5" />
                                                        {t('services.manageDNSEngine')}
                                                    </button>
                                                    ) : (
                                                    <>
                                                    {/* Inline start/stop belongs to `service` alone. A tool
                                                        (phpMyAdmin) has no unit at all; a runtime has one unit
                                                        PER VERSION, so a single Stop button would be a lie —
                                                        stop which PHP? Its controls live in the version drawer,
                                                        one row per version. Neither is a special case bolted on:
                                                        it falls out of Kind (D-010).
                                                        Satır içi başlat/durdur yalnız `service`indir. Tool'un
                                                        (phpMyAdmin) hiç unit'i yoktur; runtime'ın SÜRÜM BAŞINA
                                                        bir unit'i vardır, tek bir Durdur düğmesi yalan olurdu —
                                                        hangi PHP durdurulacak? Onun denetimi sürüm çekmecesinde,
                                                        sürüm başına bir satırdadır. İkisi de sonradan eklenmiş
                                                        özel durum değil: Kind'den düşer (D-010). */}
                                                    {/* Contextual: only actions that make sense for the current
                                                        state — no Stop on a stopped service, no Start on a running
                                                        one. / Bağlama duyarlı: yalnız mevcut duruma anlamlı gelen
                                                        eylemler — durmuşta Durdur, çalışanda Başlat gösterilmez. */}
                                                    {s.kind === 'service' && (running ? (
                                                    <>
                                                    <ActionIcon
                                                        title={t('services.restart')}
                                                        onClick={() => requestComponentAction({ kind: 'service', service: s, action: 'restart' })}
                                                        disabled={busy === s.id}
                                                        tone="warning"
                                                    >
                                                        <RotateCw className="h-4 w-4" />
                                                    </ActionIcon>
                                                    <ActionIcon
                                                        title={t('services.stop')}
                                                        onClick={() => requestComponentAction({ kind: 'service', service: s, action: 'stop' })}
                                                        disabled={busy === s.id}
                                                        tone="danger"
                                                    >
                                                        {/* A stop sign in a circle: the empty square of
                                                            alpha.94 read as a checkbox on the owner's
                                                            screens (2026-10-17).
                                                            Daire içinde dur işareti; boş kare onay kutusu
                                                            gibi okunuyordu. */}
                                                        <StopCircle className="h-4 w-4" aria-hidden="true" />
                                                    </ActionIcon>
                                                    </>
                                                    ) : s.not_offered ? (
                                                    /* A legacy installation whose automatic integration is
                                                       deliberately closed must remain manageable. Starting it
                                                       is the only honest action because Repair is not supported. */
                                                    <ActionIcon
                                                        title={t('services.start')}
                                                        onClick={() => requestComponentAction({ kind: 'service', service: s, action: 'start' })}
                                                        disabled={busy === s.id}
                                                        tone="success"
                                                    >
                                                        <Play className="h-4 w-4" aria-hidden="true" />
                                                    </ActionIcon>
                                                    ) : (
                                                    /* A stopped supported service may be a partial install.
                                                       Re-running the durable idempotent install path repairs
                                                       packages, configuration, helpers and readiness under the
                                                       same full-page operation lock. */
                                                    <ActionIcon
                                                        title={s.repair_available ? t('services.repair') : t('services.repairUnavailable')}
                                                        onClick={() => requestComponentAction({ kind: 'repair', service: s })}
                                                        disabled={busy === s.id || !s.repair_available}
                                                        tone="warning"
                                                    >
                                                        <RotateCw className="h-4 w-4" />
                                                    </ActionIcon>
                                                    ))}
                                                    {/* Roundcube repair reuses the durable install operation. It is
                                                        an ordinary action, drawn like Manage; amber only when the
                                                        server reports a webmail problem (2026-10-16).
                                                        Roundcube onarımı olağan bir eylemdir; yalnız sunucu webmail
                                                        sorunu bildirdiğinde sarı çizilir. */}
                                                    {/* Only when the server reports a webmail problem;
                                                        otherwise webmail repair lives in the Webmail
                                                        bundle's dialogue (alpha.95).
                                                        Yalnız sunucu webmail sorunu bildirdiğinde;
                                                        yoksa onarım Web postası paketinin penceresindedir. */}
                                                    {s.kind === 'tool' && s.id === 'roundcube' && webmailProblem && (
                                                    <WebmailRepairButton
                                                        problem={webmailProblem}
                                                        onClick={() => requestComponentAction({ kind: 'repair', service: s })}
                                                        disabled={mutationControlsDisabled || busy === s.id || !s.repair_available}
                                                        title={s.repair_available ? t('services.repairWebmail') : t('services.repairUnavailable')}
                                                        label={t('services.repairWebmail')}
                                                    />
                                                    )}
                                                    {/* The version drawer is where a runtime's per-version
                                                        state and controls live (B3b) — and, for a tarball
                                                        runtime, where installing a version lives too.
                                                        Sürüm çekmecesi, runtime'ın sürüm başına durum ve
                                                        denetiminin yaşadığı yerdir (B3b) — tarball runtime'da
                                                        sürüm kurmak da orada yaşar. */}
                                                    {s.kind === 'runtime' && (
                                                    <button
                                                        onClick={() => toggleDrawer(s.id)}
                                                        className={`ml-1 ${rowButtonClass}`}
                                                    >
                                                        {openDrawers.has(s.id) ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                                                        {t('services.versionsToggle')}
                                                        <span className="rounded bg-surface-2 px-1 font-mono text-xs text-fg-subtle">{(s.instances ?? []).filter((i) => i.managed).length}</span>
                                                    </button>
                                                    )}
                                                    {/* Manage: extensions/config pages. A tool has nothing to
                                                        manage, except a PHP tool, whose PHP version is chosen
                                                        there (alpha.104). / Yönet: uzantı/ayar sayfaları.
                                                        Tool'un yönetecek şeyi yoktur; PHP aracının PHP sürümü
                                                        orada seçilir. */}
                                                    {(s.kind !== 'tool' || phpToolForService(s.id) !== null) && (
                                                    <button
                                                        onClick={() => onManageService?.(s.id, s.versions)}
                                                        className={`ml-1 ${rowButtonClass}`}
                                                    >
                                                        <Settings className="h-3.5 w-3.5" />
                                                        {t('services.manage')}
                                                    </button>
                                                    )}
                                                    {/* A tarball runtime has no whole-service package to
                                                        purge; removal is per version and lands with the
                                                        usage ledger (B3d) — no delete offered until it can
                                                        refuse "in use by 3 sites".
                                                        Tarball runtime'ın purge edilecek bütün-servis paketi
                                                        yok; kaldırma sürüm başınadır ve kullanım defteriyle
                                                        gelir (B3d) — "3 site kullanıyor" diye reddedemeyen
                                                        silme sunulmaz. */}
                                                    {!isTarballRuntime(s) && (
                                                    <ActionIcon
                                                        title={t('services.uninstall')}
                                                        onClick={() => openRemovalFlow(s)}
                                                        tone="danger"
                                                    >
                                                        <Trash2 className="h-4 w-4" aria-hidden="true" />
                                                    </ActionIcon>
                                                    )}
                                                    </>
                                                    )}
                                                    </fieldset>
                                                    {s.kind === 'runtime' && openDrawers.has(s.id) && (
                                                        <div className="basis-full md:col-span-3">
                                                        <VersionDrawer
                                                            service={s}
                                                            busy={busy}
                                                            mutationDisabled={mutationControlsDisabled}
                                                            onInstanceAction={(serviceId, unit, action) => {
                                                                const target = services.find((candidate) => candidate.id === serviceId);
                                                                if (target) requestComponentAction({ kind: 'instance', service: target, unit, action });
                                                            }}
                                                            onVersionInstalled={scan}
                                                            onStateUnverified={handleStateRefreshFailure}
                                                            onInstallPackage={(pkg, version) => {
                                                                requestComponentAction({ kind: 'install-package', service: s, pkg, version });
                                                                return Promise.resolve(false);
                                                            }}
                                                            onInstallNode={(version) => {
                                                                requestComponentAction({ kind: 'install-node', service: s, version });
                                                                return Promise.resolve(false);
                                                            }}
                                                            removeRequest={versionRemoveRequest?.serviceId === s.id ? versionRemoveRequest : null}
                                                            onRemoveRequestTaken={() => setVersionRemoveRequest(null)}
                                                            onRemoveComponent={removeComponentById}
                                                            onRemoveWhole={() => openUninstallDialog(s)}
                                                        />
                                                        </div>
                                                    )}
                                                </li>
                                            );
                                        })}
                                    </ul>
                                )}
                            </section>
                        );
                    })}
                </div>
                )}
                </>
            )}

            <fieldset
                disabled={mutationControlsDisabled}
                aria-busy={pageControlsBusy}
                className="min-w-0 border-0 p-0"
            >
                {installTarget && (
                    <InstallServiceDialog
                        service={installTarget}
                        busy={busy === installTarget.id}
                        onCancel={() => setInstallTarget(null)}
                        onConfirm={(pkg, confirmed) => doInstall(installTarget, pkg, confirmed)}
                    />
                )}
                {profileTarget && (
                    <MailProfileInstallDialog
                        profile={profileTarget}
                        services={services}
                        mailHostname={mailHostname}
                        repair={profileRepair}
                        freshInstall={profileFreshInstall}
                        onCancel={() => setProfileTarget(null)}
                        onConfirm={(chosen) => void installProfile(chosen)}
                        throughSetup={mailThroughSetup ? addProfileThroughSetup : undefined}
                    />
                )}
                {bundleTarget && (
                    <BundleInstallDialog
                        bundle={bundleTarget}
                        services={services}
                        continuing={bundleSequence.run?.bundleId === bundleTarget.id && bundleSequence.run.phase === 'stopped'}
                        previous={bundleSequence.run?.bundleId === bundleTarget.id && bundleSequence.run.phase === 'stopped' ? bundleSequence.run.choices : undefined}
                        onCancel={() => setBundleTarget(null)}
                        onConfirm={confirmComponentBundle}
                    />
                )}
                {versionChoice && (
                    <RemoveWhichVersionDialog
                        service={versionChoice}
                        onCancel={() => setVersionChoice(null)}
                        onPickVersion={(version) => chooseVersionRemoval(versionChoice, version)}
                        onPickAll={() => { const target = versionChoice; setVersionChoice(null); openUninstallDialog(target); }}
                    />
                )}
                {uninstallTarget && (
                    <UninstallServiceDialog
                        key={uninstallTarget.id}
                        service={uninstallTarget}
                        busy={busy === uninstallTarget.id}
                        error={uninstallError}
                        retryCleanup={retryCleanup}
                        cleanupAttempt={cleanupAttempt}
                        onCancel={closeUninstallDialog}
                        onConfirm={() => doUninstall(uninstallTarget)}
                        onRemoveComponent={busy === uninstallTarget.id ? undefined : removeComponentById}
                        nameOf={(id) => componentName(services.find((service) => service.id === id) ?? id, t)}
                        bundleNameOf={(id, nameCode) => {
                            const bundle = bundles.find((candidate) => candidate.id === id);
                            if (bundle) return bundleName(bundle, t, (componentId) => componentName(services.find((service) => service.id === componentId) ?? componentId, t));
                            const key = `services.bundles.name.${nameCode}` as TranslationKey;
                            const said = t(key);
                            return said === key ? t('services.removal.bundleUnnamed') : said;
                        }}
                    />
                )}
            </fieldset>
            {pendingAction && (
                <ComponentActionConfirmationDialog
                    action={pendingAction}
                    confirmDisabled={hostMutationReadiness?.ready !== true || stateUnverified}
                    onCancel={() => setPendingAction(null)}
                    onConfirm={() => void confirmComponentAction()}
                />
            )}
        </div>
    );
}

// Lifecycle, repair and extra-runtime changes never run from the first click.
// This dialog names the target and immediate effect; package installs that
// need richer package/version detail keep their dedicated dialogs.
// Yaşam döngüsü, onarım ve ek runtime değişiklikleri ilk tıklamayla çalışmaz.
// Bu pencere hedefi ve doğrudan etkiyi açıklar; paket/sürüm ayrıntısı gereken
// kurulumlar kendi ayrıntılı pencerelerini kullanmaya devam eder.
function ComponentActionConfirmationDialog({
    action,
    confirmDisabled,
    onCancel,
    onConfirm,
}: {
    action: PendingComponentAction;
    confirmDisabled: boolean;
    onCancel: () => void;
    onConfirm: () => void;
}) {
    const { t } = useI18n();
    const actionName = action.kind === 'service' || action.kind === 'instance'
        ? action.action
        : action.kind === 'repair'
            ? 'repair'
            : 'install';
    const targetName = action.kind === 'instance'
        ? `${componentName(action.service, t)} (${action.unit})`
        : action.kind === 'install-package'
            ? `${componentName(action.service, t)} ${action.version}`
            : action.kind === 'install-node'
                ? `Node ${action.version}`
                : componentName(action.service, t);
    const icon = actionName === 'start'
        ? Play
        : actionName === 'stop'
            ? StopCircle
            : actionName === 'install'
                ? DownloadCloud
                : RotateCw;
    const Icon = icon;

    return (
        <Dialog
            id='component-action-confirm'
            iconSlot={
                <span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${
                    actionName === 'stop' ? 'bg-danger/10 text-danger' : 'bg-primary/10 text-primary'
                }`}>
                    <Icon className='h-5 w-5' fill={actionName === 'start' || actionName === 'stop' ? 'currentColor' : 'none'} />
                </span>
            }
            title={t(`services.confirm.${actionName}.title` as Parameters<typeof t>[0], { name: targetName })}
            description={t(`services.confirm.${actionName}.description` as Parameters<typeof t>[0], { name: targetName })}
            onDismiss={onCancel}
            actions={
                <>
                    <Button variant='secondary' autoFocus onClick={onCancel}>
                        {t('common.cancel')}
                    </Button>
                    <Button
                        variant={actionName === 'stop' ? 'danger' : 'primary'}
                        icon={icon}
                        disabled={confirmDisabled}
                        onClick={onConfirm}
                    >
                        {t(`services.confirm.${actionName}.button` as Parameters<typeof t>[0], { name: targetName })}
                    </Button>
                </>
            }
        />
    );
}

function MailProfileInstallDialog({
    profile,
    services,
    mailHostname,
    repair = false,
    freshInstall = false,
    onCancel,
    onConfirm,
    throughSetup,
}: {
    profile: ManagedMailProfile;
    services: ManagedService[];
    mailHostname: MailHostnameIdentity | null;
    onCancel: () => void;
    onConfirm: (mailHostname: string) => void;
    /** Opened from a bundle's Repair: the repair form, whatever the state. */
    repair?: boolean;
    /** Opened from a bundle chip that says Install: the install form. */
    freshInstall?: boolean;
    // Present on a server whose guided setup has finished: the profile is
    // added to an extension of that setup and reviewed there (2026-10-16).
    throughSetup?: (mailHostname: string) => Promise<TranslationKey | null>;
}) {
    const { t } = useI18n();
    const [acknowledged, setAcknowledged] = useState(false);
    // A saved name can be corrected here (2026-10-16). What changes with it is
    // said before anything is confirmed.
    // Kayitli ad burada duzeltilebilir; neyin degisecegi onaydan once soylenir.
    const [editingName, setEditingName] = useState(false);
    const [routeBusy, setRouteBusy] = useState(false);
    const [routeError, setRouteError] = useState<TranslationKey | null>(null);
    // A refusal is said under the name, at the end of the dialog's scrolling
    // body; on a phone that is below what is shown, so it is brought into view
    // (2026-10-16). / Ret nedeni gorunur alana alinir.
    const routeErrorRef = useRef<HTMLParagraphElement>(null);
    useEffect(() => {
        if (routeError) routeErrorRef.current?.scrollIntoView?.({ block: 'nearest' });
    }, [routeError]);
    // The server is only asked for a name when it does not already carry a
    // fully qualified one. A server that does is stated as a fact and left
    // alone. / Sunucudan bir ad yalniz zaten tam nitelikli bir ad tasimiyorsa
    // istenir. Tasiyan bir sunucu bir olgu olarak soylenir ve ona dokunulmaz.
    const hostnameSettled = mailHostname?.source === 'saved' && canonicalMailHostname(mailHostname.hostname) !== null;
    const [hostnameDraft, setHostnameDraft] = useState(mailHostname?.hostname ?? '');
    const [hostnameTouched, setHostnameTouched] = useState(false);
    const canonicalHostname = canonicalMailHostname(hostnameDraft);
    const hostnameInvalid = hostnameDraft.trim() !== '' && canonicalHostname === null;
    const nameFixed = hostnameSettled && !editingName;
    const hostnameReady = nameFixed || canonicalHostname !== null;
    const savedName = hostnameSettled ? mailHostname?.hostname ?? '' : '';
    const renaming = hostnameSettled && editingName && canonicalHostname !== null && canonicalHostname !== savedName;
    const chosenName = nameFixed ? '' : canonicalHostname ?? '';
    const reviewInSetup = async () => {
        if (!throughSetup || routeBusy || !hostnameReady) return;
        setRouteBusy(true); setRouteError(null);
        const problem = await throughSetup(chosenName === savedName ? '' : chosenName);
        setRouteBusy(false);
        if (problem) setRouteError(problem);
    };
    // Only a name the panel derived is worth attributing; a name that simply
    // came from the operating system is not a suggestion.
    // Yalniz panelin turettigi bir ad kaynagiyla anilmaya deger; isletim
    // sisteminden gelen bir ad bir oneri degildir.
    const suggestionSource = mailHostname
        && mailHostname.source !== ''
        && mailHostname.source !== 'os_hostname'
        ? mailHostname.source
        : null;
    const hostnameInputRef = useRef<HTMLInputElement>(null);
    const [candidateVersions, setCandidateVersions] = useState<Record<string, string>>({});
    const [candidatesLoading, setCandidatesLoading] = useState(true);
    const acknowledgementRef = useRef<HTMLInputElement>(null);
    const components = profile.services
        .map((id) => services.find((service) => service.id === id))
        .filter((service): service is ManagedService => Boolean(service));
    const mode = repair ? 'repair' : freshInstall || profile.status === 'available'
        ? 'install'
        : profile.status === 'partial'
            ? 'continue'
            : 'repair';
    const ports = Array.from(new Set(components.flatMap((service) => service.ports ?? []))).sort();
    const restarted = components
        .filter((service) => service.kind === 'service')
        .map((service) => componentName(service, t));

    useEffect(() => {
        // The first thing to do is the first thing focused: on a server that
        // still needs a name, that is the name field.
        // Yapilacak ilk sey, odaklanan ilk seydir: hala bir ada ihtiyaci olan
        // bir sunucuda bu, ad alanidir.
        // Escape belongs to the shared dialogue now, so it is not repeated here.
        // Escape artik paylasilan diyaloga aittir; burada tekrarlanmaz.
        if (hostnameInputRef.current) hostnameInputRef.current.focus();
        else acknowledgementRef.current?.focus();
    }, []);

    useEffect(() => {
        let cancelled = false;
        const packageComponents = components.filter((service) =>
            !service.is_installed && (service.packages?.length ?? 0) > 0);
        if (packageComponents.length === 0) {
            setCandidateVersions({});
            setCandidatesLoading(false);
            return () => {
                cancelled = true;
            };
        }
        setCandidatesLoading(true);
        void Promise.all(packageComponents.map(async (service) => {
            try {
                const response = await fetch(`/api/v1/service/candidate?id=${encodeURIComponent(service.id)}`);
                if (!response.ok) return [service.id, ''] as const;
                const value: unknown = await response.json();
                if (!value || typeof value !== 'object') return [service.id, ''] as const;
                const version = (value as Record<string, unknown>).version;
                return [service.id, typeof version === 'string' ? version : ''] as const;
            } catch {
                return [service.id, ''] as const;
            }
        })).then((entries) => {
            if (!cancelled) setCandidateVersions(Object.fromEntries(entries));
        }).finally(() => {
            if (!cancelled) setCandidatesLoading(false);
        });
        return () => {
            cancelled = true;
        };
        // The dialog is bound to one immutable verified catalogue snapshot.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [profile.id]);

    const componentVersion = (service: ManagedService) => {
        if (service.is_installed) {
            return service.versions.length > 0
                ? service.versions.join(', ')
                : t('services.mailProfiles.plan.installed');
        }
        if (service.id === 'roundcube') return '1.6.20';
        if (candidatesLoading) return '…';
        return candidateVersions[service.id] || t('services.mailProfiles.plan.repositoryCandidate');
    };
    const componentPackages = (service: ManagedService) => {
        if ((service.packages?.length ?? 0) > 0) return service.packages!.join(', ');
        if (service.id === 'roundcube') return 'Roundcube 1.6.20';
        return service.id;
    };

    return (
        <Dialog
            id='mail-profile-confirm'
            width='xl'
            title={t(`services.mailProfiles.plan.title.${mode}` as Parameters<typeof t>[0], { name: mailProfileName(profile, t) })}
            description={t('services.mailProfiles.plan.description')}
            onDismiss={onCancel}
            // While setup is asked to take the profile, the dialog stays and
            // nothing in it can change what was sent (2026-10-16).
            busy={routeBusy}
            footerLead={throughSetup ? undefined :
                <label className='flex cursor-pointer items-start gap-3 rounded-lg border border-border p-3'>
                    <input
                        ref={acknowledgementRef}
                        type='checkbox'
                        checked={acknowledged}
                        onChange={(event) => setAcknowledged(event.target.checked)}
                        className='mt-0.5 h-4 w-4 rounded-md border-border-strong text-primary'
                    />
                    <span className='text-sm leading-5 text-fg'>
                        {t('services.mailProfiles.plan.acknowledgement')}
                    </span>
                </label>
            }
            actions={
                <>
                    <Button variant='secondary' disabled={routeBusy} onClick={onCancel}>
                        {t('common.cancel')}
                    </Button>
                    {throughSetup ? (
                        <Button
                            variant='primary'
                            disabled={!hostnameReady || routeBusy}
                            loading={routeBusy}
                            onClick={() => void reviewInSetup()}
                        >
                            {t('services.mailProfiles.setupRoute.confirm')}
                        </Button>
                    ) : (
                        <Button
                            variant='primary'
                            icon={mode === 'install' ? DownloadCloud : RotateCw}
                            disabled={!acknowledged || !hostnameReady}
                            onClick={() => onConfirm(chosenName === savedName ? '' : chosenName)}
                        >
                            {t(`services.mailProfiles.plan.confirm.${mode}` as Parameters<typeof t>[0], { name: mailProfileName(profile, t) })}
                        </Button>
                    )}
                </>
            }
        >
                    <div className='mb-4 overflow-hidden rounded-xl border border-border'>
                        <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-3 bg-surface-2 px-4 py-2 text-xs font-semibold text-fg-subtle'>
                            <span>{t('services.mailProfiles.plan.component')}</span>
                            <span>{t('services.mailProfiles.plan.version')}</span>
                        </div>
                        {components.map((service) => (
                            <div key={service.id} className='grid grid-cols-[minmax(0,1fr)_auto] gap-3 border-t border-border px-4 py-3'>
                                <div className='min-w-0'>
                                    <p className='text-sm font-medium text-fg'>{componentName(service, t)}</p>
                                    <p className='break-words font-mono text-xs text-fg-subtle'>{componentPackages(service)}</p>
                                </div>
                                <span className='max-w-52 text-right font-mono text-xs font-medium text-fg'>
                                    {componentVersion(service)}
                                </span>
                            </div>
                        ))}
                    </div>

                    <div className='mb-4 grid gap-3 sm:grid-cols-2'>
                        <div className='rounded-lg border border-border bg-surface-2/50 p-3'>
                            <p className='text-xs font-semibold text-fg'>{t('services.mailProfiles.plan.serviceImpact')}</p>
                            <p className='mt-1 text-xs leading-5 text-fg-muted'>
                                {restarted.length > 0
                                    ? t('services.mailProfiles.plan.restarts', { services: restarted.join(', ') })
                                    : t('services.mailProfiles.plan.noRestarts')}
                            </p>
                        </div>
                        <div className='rounded-lg border border-border bg-surface-2/50 p-3'>
                            <p className='text-xs font-semibold text-fg'>{t('services.mailProfiles.plan.firewallImpact')}</p>
                            <p className='mt-1 text-xs leading-5 text-fg-muted'>
                                {ports.length > 0
                                    ? t('services.mailProfiles.plan.ports', { ports: ports.join(', ') })
                                    : t('services.mailProfiles.plan.noPorts')}
                            </p>
                        </div>
                    </div>

                    {mode === 'repair' && (
                        // What Repair does, said before it is pressed: there is
                        // no read-only verification of a mail setup.
                        // Onar'ın ne yaptığı basılmadan söylenir.
                        <p className='mb-3 text-sm leading-6 text-fg' data-mail-profile-repair-hint=''>{t('services.mailProfiles.plan.repairHint')}</p>
                    )}
                    {throughSetup ? (
                        <div className='mb-4 rounded-lg border border-border bg-surface-2/50 p-3 text-xs leading-5 text-fg' data-mail-profile-route='setup'>
                            <p>{t('services.mailProfiles.setupRoute.note')}</p>
                        </div>
                    ) : (
                        <div className='mb-4 rounded-lg border border-warning-mark/60 bg-warning-mark/10 p-3 text-xs leading-5 text-fg-muted' data-mail-profile-route='direct'>
                            <p>{t('services.mailProfiles.plan.tls')}</p>
                            <p className='mt-1'>{t('services.mailProfiles.plan.partialProgress')}</p>
                            <p className='mt-1'>{t('services.mailProfiles.directRoute.note')}</p>
                        </div>
                    )}

                    <section className='mb-4 rounded-lg border border-border p-3'>
                        <p className='text-xs font-semibold text-fg'>
                            {t('services.mailProfiles.hostname.title')}
                        </p>
                        {nameFixed ? (
                            <div className='mt-1 flex flex-wrap items-baseline gap-x-3 gap-y-1'>
                                <p className='min-w-0 flex-1 text-xs leading-5 text-fg-muted'>
                                    {t('services.mailProfiles.hostname.settled', {
                                        hostname: mailHostname?.hostname ?? '',
                                    })}
                                </p>
                                <button
                                    type='button'
                                    data-mail-hostname-change=''
                                    disabled={routeBusy}
                                    onClick={() => { setHostnameDraft(savedName); setEditingName(true); }}
                                    className='min-h-9 text-xs font-medium text-primary underline underline-offset-4 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary disabled:cursor-not-allowed disabled:opacity-60'
                                >
                                    {t('services.mailProfiles.hostname.change')}
                                </button>
                            </div>
                        ) : (
                            <>
                                <p id='mail-profile-hostname-hint' className='mt-1 text-xs leading-5 text-fg-muted'>
                                    {mailHostname?.current
                                        ? t('services.mailProfiles.hostname.hint', {
                                            current: mailHostname.current,
                                        })
                                        : t('services.mailProfiles.hostname.hintUnnamed')}
                                </p>
                                <label
                                    htmlFor='mail-profile-hostname'
                                    className='mt-3 block text-xs font-medium text-fg-muted'
                                >
                                    {t('services.mailProfiles.hostname.label')}
                                </label>
                                <input
                                    id='mail-profile-hostname'
                                    ref={hostnameInputRef}
                                    type='text'
                                    inputMode='url'
                                    autoComplete='off'
                                    spellCheck={false}
                                    value={hostnameDraft}
                                    disabled={routeBusy}
                                    placeholder={t('services.mailProfiles.hostname.placeholder')}
                                    aria-describedby='mail-profile-hostname-hint mail-profile-hostname-note'
                                    aria-invalid={hostnameInvalid || undefined}
                                    onChange={(event) => {
                                        setHostnameDraft(event.target.value);
                                        setHostnameTouched(true);
                                    }}
                                    className={`mt-1 w-full rounded-lg border bg-surface px-3 py-1.5 font-mono text-sm text-fg placeholder:text-fg-subtle ${
                                        hostnameInvalid && hostnameTouched
                                            ? 'border-danger focus:border-danger'
                                            : 'border-border-strong focus:border-primary'
                                    }`}
                                />
                                {/* While a saved name is being changed, the note under the field
                                    says what the change does, so the field's description is that
                                    sentence (2026-10-16). / Ad degisirken alanin aciklamasi etkidir. */}
                                <p
                                    id='mail-profile-hostname-note'
                                    {...(renaming ? { 'data-mail-hostname-effects': '' } : {})}
                                    className={`mt-1 text-xs leading-5 ${
                                        hostnameInvalid && hostnameTouched ? 'text-danger' : renaming ? 'text-fg' : 'text-fg-muted'
                                    }`}
                                >
                                    {hostnameInvalid && hostnameTouched
                                        ? t('services.mailProfiles.hostname.invalid')
                                        : renaming
                                            ? t('services.mailProfiles.hostname.changeEffects', { old: savedName, new: canonicalHostname ?? '' })
                                            : canonicalHostname
                                                ? t('services.mailProfiles.hostname.rename', {
                                                    hostname: canonicalHostname,
                                                })
                                                : t('services.mailProfiles.hostname.invalid')}
                                </p>
                                {hostnameSettled && editingName && (
                                    <button
                                        type='button'
                                        disabled={routeBusy}
                                        onClick={() => { setEditingName(false); setHostnameDraft(savedName); setHostnameTouched(false); }}
                                        className='mt-2 min-h-9 text-xs font-medium text-primary underline underline-offset-4 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary disabled:cursor-not-allowed disabled:opacity-60'
                                    >
                                        {t('services.mailProfiles.hostname.keep', { hostname: savedName })}
                                    </button>
                                )}
                                {suggestionSource && !editingName && canonicalHostname === mailHostname?.hostname && (
                                    // fg-subtle is the placeholder colour and carries no
                                    // meaning; where the name came from is information.
                                    // fg-subtle yer tutucu rengidir ve bir anlam taşımaz;
                                    // adın nereden geldiği ise bilgidir.
                                    <p className='mt-1 text-xs leading-5 text-fg-muted'>
                                        {t(`services.mailProfiles.hostname.source.${suggestionSource}` as Parameters<typeof t>[0])}
                                    </p>
                                )}
                            </>
                        )}
                    </section>
                    {routeError && (
                        <p ref={routeErrorRef} role='alert' className='mb-2 text-sm leading-6 text-fg' data-mail-profile-route-error=''>{t(routeError)}</p>
                    )}
        </Dialog>
    );
}

function InstallServiceDialog({
    service,
    busy,
    onCancel,
    onConfirm,
}: {
    service: ManagedService;
    busy: boolean;
    onCancel: () => void;
    onConfirm: (pkg: string | undefined, confirmed: string[]) => void;
}) {
    const { t } = useI18n();
    // What the package manager would install right now, as the server reports
    // it: '' is the server saying it names no version; a read that failed is
    // neither a version nor "the distribution default".
    // Paket yöneticisinin şu an kuracağı, sunucunun bildirdiği hâliyle: ''
    // sunucunun sürüm adlandırmadığını söylemesidir; başarısız okuma ne sürümdür
    // ne de "dağıtım varsayılanı".
    const candidate = useRemote(`/api/v1/service/candidate?id=${encodeURIComponent(service.id)}`, decodeCandidateVersion);
    const version = candidate.remote.state === 'known' ? candidate.remote.value : '';

    // Managed vendor repo (e.g. PGDG): most services have none, so this whole
    // section simply does not render. When present, the admin can enable it and
    // pick a specific major version instead of the single one the distro froze.
    // selectedPkg === '' means "distro default" (install from the OS repo).
    // Yönetilen vendor deposu (örn. PGDG): çoğu servisin yok, o durumda bu bölüm
    // hiç render edilmez. Varsa, yönetici açıp dağıtımın dondurduğu tek sürüm
    // yerine belirli bir major seçebilir. selectedPkg === '' → "dağıtım
    // varsayılanı" (OS deposundan kur).
    const repoRead = useRemote(`/api/v1/repo?service_id=${encodeURIComponent(service.id)}`, decodeRepoInfo);
    // The repository block is drawn from the last answer; installing is offered
    // only on the answer as it is now. Until the server has said whether this
    // component needs a repository, and what state it is in, Install is off.
    // Depo bölümü son yanıttan çizilir; kurulum yalnız şimdiki yanıt üzerinde
    // sunulur. Sunucu bu bileşenin depoya ihtiyacı olup olmadığını ve deponun
    // durumunu söyleyene dek Kur kapalıdır.
    const repoObserved = lastKnown(repoRead.remote)?.value ?? null;
    const repo = repoObserved?.available ? repoObserved : null;
    const repoKnown = repoRead.remote.state === 'known' && !repoRead.reading;
    const [repoBusy, setRepoBusy] = useState(false);
    const [repoAction, setRepoAction] = useState<'enable' | 'disable' | null>(null);
    const [selectedPkg, setSelectedPkg] = useState<string>('');
    const repoError = repo?.error_code ? apiErrorText({ message: '', code: repo.error_code }, t, 'services.actionFailed') : '';

    // Enable/disable the vendor repo, then reflect the new state (and the
    // versions it now exposes) straight from the server's reply.
    // Vendor deposunu aç/kapat, sonra yeni durumu (ve açtığı sürümleri) doğrudan
    // sunucunun yanıtından yansıt.
    const toggleRepo = async (action: 'enable' | 'disable') => {
        setRepoBusy(true);
        try {
            const res = await fetch('/api/v1/repo', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ service_id: service.id, action }),
            });
            if (!res.ok) {
                showToast('error', apiErrorText(await readApiError(res), t, 'services.actionFailed'));
                return;
            }
            if (action === 'disable') setSelectedPkg('');
            // The state shown is the one the server reports after the change.
            await repoRead.retry();
        } catch {
            // No answer: whether the repository was changed is not known. It is
            // read again and nothing is sent a second time.
            // Yanıt yok: deponun değişip değişmediği bilinmiyor. Yeniden okunur;
            // hiçbir şey ikinci kez gönderilmez.
            showToast('error', t('common.resultUnknown'));
            await repoRead.retry();
        } finally {
            setRepoBusy(false);
        }
    };

    // alpha.105: the package manager's simulation of this install, read while
    // the dialogue is open; notable packages need the owner's confirmation.
    // Bu kurulumun benzetimi; dikkat çeken paketler onay ister.
    const previewURL = service.id === 'roundcube' ? null : installPreviewURL([{ id: service.id, ...(selectedPkg ? { package: selectedPkg } : {}) }]);
    const preview = useRemote(previewURL, decodeInstallPreview);
    const [acknowledgedFor, setAcknowledgedFor] = useState<string | null>(null);
    const ackKey = installPreviewAckKey(previewURL, preview);
    const previewGate = installPreviewGate(previewURL ? preview : null, ackKey !== null && acknowledgedFor === ackKey);

    const willInstall = selectedPkg
        ? [selectedPkg]
        : service.packages && service.packages.length > 0
          ? service.packages
          : [service.id];

    return (
        <>
            <Dialog
                id="service-install"
                icon={componentIcon(service.id, service.category)}
                title={t('services.installTitle', { name: componentName(service, t) })}
                description={componentDescription(service, t)}
                busy={busy || repoBusy}
                onDismiss={onCancel}
                actions={
                    <>
                        <Button variant="secondary" onClick={onCancel} disabled={busy || repoBusy}>{t('common.cancel')}</Button>
                        <Button
                            variant="primary"
                            onClick={() => onConfirm(selectedPkg || undefined, previewGate.confirmed)}
                            disabled={busy || repoBusy || !repoKnown || previewGate.hold || Boolean(repo?.required && (repo.error_code || !repo.enabled))}
                            icon={DownloadCloud}
                        >
                            {busy ? t('services.installing') : t('services.install')}
                        </Button>
                    </>
                }
            >
                <div className="mb-4 rounded-lg border border-border bg-surface-2/50 p-3">
                    <div className="mb-2 flex items-center justify-between">
                        <span className="text-xs font-medium text-fg-subtle">{t('services.versionToInstall')}</span>
                        {/* A version is a literal and keeps the mono face; "could
                            not be checked" and "distribution default" are words.
                            Sürüm bir değerdir ve tek aralıklı yazılır; "kontrol
                            edilemedi" ve "dağıtım varsayılanı" sözcüktür. */}
                        {selectedPkg || candidate.remote.state === 'loading' || version ? (
                            <span className="font-mono text-sm font-semibold text-fg">
                                {selectedPkg || (candidate.remote.state === 'loading' ? '…' : version)}
                            </span>
                        ) : (
                            <span className="text-sm font-semibold text-fg">
                                {t(candidate.remote.state === 'known' ? 'services.versionDefault' : 'services.versionUnknown')}
                            </span>
                        )}
                    </div>
                    <p className="mb-1 text-xs font-medium text-fg-subtle">{t('services.willInstall')}</p>
                    <div className="flex flex-wrap gap-1.5">
                        {willInstall.map((pkg) => (
                            <span key={pkg} className="rounded bg-surface-3 px-2 py-0.5 font-mono text-xs text-fg">{pkg}</span>
                        ))}
                    </div>
                    {service.id === 'php-fpm' && (
                        <p className="mt-2 text-xs text-fg-subtle">{t('services.phpVersionNote')}</p>
                    )}
                    {previewURL && (
                        <InstallPreviewNotes
                            handle={preview}
                            acknowledged={ackKey !== null && acknowledgedFor === ackKey}
                            onAcknowledge={(value) => setAcknowledgedFor(value ? ackKey : null)}
                            componentLabel={(id) => componentName(id, t)}
                            avoidable={Boolean(selectedPkg)}
                        />
                    )}
                </div>

                {repoRead.remote.state === 'loading' && <Checking label={t('services.repo.checking')} className="mb-4" />}
                {repoRead.remote.state === 'unknown' && (
                    <CouldNotCheck
                        className="mb-4"
                        text={t(repoObserved ? 'services.repo.stale' : 'services.repo.unknown', { name: componentName(service, t) })}
                        onRetry={() => void repoRead.retry()}
                        busy={repoRead.reading}
                    />
                )}
                {repo && (
                    <div className="mb-4 rounded-lg border border-border bg-surface-2/50 p-3">
                        <div className="mb-2 flex items-start gap-2">
                            <Layers className="mt-0.5 h-4 w-4 shrink-0 text-fg-subtle" />
                            <div className="min-w-0">
                                <p className="text-sm font-medium text-fg">{repo.name}</p>
                                <p className="text-xs text-fg-subtle">{repo.detail}</p>
                            </div>
                        </div>

                        {!repo.enabled ? (
                            <>
                                {/* Required repo (Netdata on Debian/Ubuntu): the
                                    package exists NOWHERE else, so pressing Install
                                    without it fails inside apt with "no installation
                                    candidate" — the operator's failed attempt on
                                    25 Jul. Say it up front instead.
                                    Zorunlu depo (Debian/Ubuntu'da Netdata): paket
                                    BAŞKA HİÇBİR yerde yok; onsuz Kur'a basmak apt
                                    içinde "kurulum adayı yok" ile düşer — operatörün
                                    25 Tem'deki başarısız denemesi. Bunu baştan söyle. */}
                                <p className={`mb-2 text-xs ${repo.required ? 'text-warning' : 'text-fg-subtle'}`}>
                                    {repo.required ? t('services.repo.requiredNote') : t('services.repo.note')}
                                </p>
                                {repoError && <p className="mb-2 text-xs text-danger">{repoError}</p>}
                                <Button
                                    variant="secondary"
                                    onClick={() => setRepoAction('enable')}
                                    disabled={repoBusy || busy || !repoKnown || Boolean(repo.error_code && !repo.repairable)}
                                    loading={repoBusy}
                                    icon={Layers}
                                >
                                    {repoBusy ? t('services.repo.enabling') : t('services.repo.enable')}
                                </Button>
                            </>
                        ) : (
                            <>
                                {repoError && <p className="mb-2 text-xs text-danger">{repoError}</p>}
                                <p className="mb-1.5 text-xs font-medium text-fg-subtle">{t('services.repo.chooseVersion')}</p>
                                <div className="flex flex-wrap gap-1.5">
                                    <button
                                        onClick={() => setSelectedPkg('')}
                                        disabled={repoBusy || busy}
                                        className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                                            selectedPkg === ''
                                                ? 'bg-primary text-primary-fg'
                                                : 'bg-surface-3 text-fg-muted hover:text-fg'
                                        } disabled:cursor-not-allowed disabled:opacity-50`}
                                    >
                                        {t('services.repo.distroDefault')}
                                        {version ? ` (${version})` : ''}
                                    </button>
                                    {(repo.packages || []).map((pkg) => (
                                        <button
                                            key={pkg}
                                            onClick={() => setSelectedPkg(pkg)}
                                            disabled={repoBusy || busy}
                                            className={`rounded-md px-2.5 py-1 font-mono text-xs font-medium transition-colors ${
                                                selectedPkg === pkg
                                                    ? 'bg-primary text-primary-fg'
                                                    : 'bg-surface-3 text-fg-muted hover:text-fg'
                                            } disabled:cursor-not-allowed disabled:opacity-50`}
                                        >
                                            {pkg}
                                        </button>
                                    ))}
                                </div>
                                <button
                                    onClick={() => setRepoAction('disable')}
                                    disabled={repoBusy || busy || !repoKnown}
                                    className="mt-2 text-xs text-fg-subtle underline decoration-dotted underline-offset-2 hover:text-fg disabled:opacity-40"
                                >
                                    {repoBusy ? '…' : t('services.repo.disable')}
                                </button>
                            </>
                        )}
                    </div>
                )}
            </Dialog>
            {repoAction && repo && (
                <RepositoryActionConfirmationDialog
                    action={repoAction}
                    repositoryName={repo.name || componentName(service, t)}
                    confirmDisabled={repoBusy ? true : busy}
                    onCancel={() => setRepoAction(null)}
                    onConfirm={() => {
                        const action = repoAction;
                        setRepoAction(null);
                        void toggleRepo(action);
                    }}
                />
            )}
        </>
    );
}

// The one outlined recipe of the row actions (alpha.94, critique item 6):
// Install, Manage, Versions and the DNS engine look alike, are 44 px tall on
// a phone, and say "unavailable" with a dashed outline, as Button does.
// Satır eylemlerinin tek çerçeveli tarifi.
const rowButtonClass = 'inline-flex items-center gap-1.5 rounded-lg border border-border-strong bg-surface px-2.5 py-1.5 text-xs font-medium text-fg transition-colors hover:bg-surface-2 disabled:cursor-not-allowed disabled:border-dashed disabled:bg-transparent disabled:text-fg-muted max-sm:min-h-11 max-sm:px-3';

// The version a row says (alpha.97, D-036): the multi-version runtimes their
// managed copies' versions; every other installed component the upstream part
// of its package version, in mono, with the full version as printed by the
// package database in its tooltip; "several versions" when its packages
// differ; nothing at all when it is not known.
// Satırın söylediği sürüm: runtime'lar kopyalarını, diğerleri paket sürümünün
// kısa biçimini (tam sürüm ipucunda), farklıysa "birden çok sürüm", bilinmiyorsa
// hiçbir şey.
function RowVersion(props: { service: ManagedService; column?: boolean; className?: string }) {
    // alpha.107: the webmail row also says an update is available (D-038 §9).
    if (props.service.id === 'roundcube' && props.service.is_installed) {
        return (
            <>
                <RowVersionText {...props} />
                <RoundcubeRowUpdate />
            </>
        );
    }
    return <RowVersionText {...props} />;
}

// The Roundcube row says an update is available (alpha.107); it shares the
// webmail page's read and says nothing until that read is known.
// Roundcube satırı, güncelleme varsa söyler.
function RoundcubeRowUpdate() {
    const { t } = useI18n();
    const handle = useRemote(ROUNDCUBE_UPDATE_URL, decodeRoundcubeUpdate, { freshFor: 10 * 60_000 });
    const remote = handle.remote;
    if (remote.state !== 'known' || !remote.value.update_available || !remote.value.latest_on_line) return null;
    return (
        <div className="text-xs font-medium text-warning" data-row-webmail-update={remote.value.latest_on_line}>
            {t('life.webmail.rowAvailable', { latest: remote.value.latest_on_line })}
        </div>
    );
}

function RowVersionText({ service, column = false, className = '' }: { service: ManagedService; column?: boolean; className?: string }) {
    const { t } = useI18n();
    if (!service.is_installed) return null;
    if (service.versions.length > 0) {
        return column && service.versions.length > 1 ? (
            <div className="flex flex-wrap gap-1" data-row-version="">
                {service.versions.map((version) => (
                    <span key={version} className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-xs text-fg-muted">{version}</span>
                ))}
            </div>
        ) : (
            // Never clipped silently (alpha.101): an ellipsis, and the whole
            // text in the tooltip. / Sessizce kırpılmaz: üç nokta ve ipucu.
            <div className={`truncate font-mono text-xs text-fg-muted ${className}`} title={service.versions.join(', ')} data-row-version="">{service.versions.join(', ')}</div>
        );
    }
    const facts = packageVersionFacts(service);
    if (!facts) return null;
    const source = t(facts.kept ? 'services.versionFromPackagesKept' : 'services.versionFromPackages');
    if (facts.several) {
        return (
            <div
                className={`text-xs text-fg-muted ${className}`}
                title={`${facts.packages.map((pkg) => `${pkg.package} ${pkg.version}`).join(', ')} · ${source}`}
                data-row-version="several"
            >
                {t('services.versionSeveral')}
            </div>
        );
    }
    return (
        <div
            className={`truncate font-mono text-xs text-fg-muted ${className}`}
            title={`${facts.full} · ${source}`}
            data-row-version={facts.full}
        >
            {facts.short}
            <span className="sr-only">{` (${facts.full}, ${source})`}</span>
        </div>
    );
}

function RowIcon({ id, category, installed }: { id: string; category: string; installed: boolean }) {
    const Icon = componentIcon(id, category);
    return (
        <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ${installed ? 'bg-surface-2 text-fg-muted' : 'bg-surface-subtle text-fg-subtle'}`} aria-hidden="true">
            <Icon className="h-4 w-4" />
        </span>
    );
}

function ActionIcon({
    children,
    title,
    onClick,
    disabled,
    tone,
}: {
    children: React.ReactNode;
    title: string;
    onClick: () => void;
    disabled?: boolean;
    tone: 'success' | 'danger' | 'warning';
}) {
    // Neutral at rest, its tone only under the pointer or the keyboard focus
    // (alpha.94, critique item 1): a healthy server no longer reads as a list
    // of faults, and amber never stands beside red on a running row
    // (DESIGN.md, State Colour and No-Adjacency rules). Stop's red stays in
    // its confirmation dialogue. On a phone the name is written and the
    // target is 44 px tall.
    // Dururken nötr; ton yalnız işaretçi ya da klavye odağında. Telefonda ad
    // yazılır ve hedef 44 piksel yüksekliğindedir.
    // Restart, Stop, Repair, Start and Delete share this one shape (alpha.95,
    // owner's screens: restart and stop were borderless beside a bordered
    // delete): the same size, the same outline, neutral at rest.
    // Yeniden başlat, Durdur, Onar, Başlat ve Sil aynı biçimi paylaşır.
    const toneCls = {
        success: 'hover:bg-success/10 hover:text-success focus-visible:text-success',
        danger: 'hover:bg-danger/10 hover:text-danger focus-visible:text-danger',
        warning: 'hover:bg-warning-mark/20 hover:text-warning focus-visible:text-warning',
    }[tone];
    return (
        <button
            type="button"
            title={title}
            aria-label={title}
            onClick={onClick}
            disabled={disabled}
            className={`inline-flex items-center gap-1.5 rounded-lg border border-border-strong bg-surface p-1.5 text-xs font-medium text-fg-muted transition-colors disabled:cursor-not-allowed disabled:border-dashed disabled:bg-transparent disabled:text-fg-subtle disabled:hover:bg-transparent max-sm:min-h-11 max-sm:px-3 ${toneCls}`}
        >
            {children}
            <span className="sm:sr-only">{title}</span>
        </button>
    );
}


// Local until B4 unifies the 5+ copies across the app. / B4 uygulamadaki 5+
// kopyayı tekleştirene dek yerel.
function fmtBytes(n: number): string {
    if (n >= 1024 * 1024 * 1024) return `${(n / (1024 * 1024 * 1024)).toFixed(1)} GB`;
    if (n >= 1024 * 1024) return `${Math.round(n / (1024 * 1024))} MB`;
    return `${Math.max(1, Math.round(n / 1024))} KB`;
}

// The version drawer (B3b): a runtime's versions live INSIDE the row, one
// sub-row per installed copy — status and start/stop per unit-bearing copy
// (php8.3-fpm), size for tarball trees (node), an honest "system" badge for
// what the panel found but did not install. For a tarball runtime this is
// also the ONLY place a version is installed from — versions never multiply
// the main list (D-010), and the package dialog stays a package dialog.
//
// Sürüm çekmecesi (B3b): runtime'ın sürümleri satırın İÇİNDE yaşar, kurulu
// kopya başına bir alt satır — unit taşıyan kopyada (php8.3-fpm) durum ve
// başlat/durdur, tarball ağacında (node) boyut, panelin bulduğu ama kurmadığı
// için dürüst "sistem" rozeti. Tarball runtime için sürüm kurmanın TEK adresi
// de burasıdır — sürümler ana listeyi asla çoğaltmaz (D-010) ve paket modalı
// paket modalı olarak kalır.
function VersionDrawer({
    service,
    busy,
    mutationDisabled,
    onInstanceAction,
    onVersionInstalled,
    onStateUnverified,
    onInstallPackage,
    onInstallNode,
    removeRequest,
    onRemoveRequestTaken,
    onRemoveComponent,
    onRemoveWhole,
}: {
    service: ManagedService;
    busy: string | null;
    mutationDisabled: boolean;
    onInstanceAction: (serviceId: string, unit: string, action: 'start' | 'stop' | 'restart') => void;
    onVersionInstalled: () => Promise<boolean>;
    onStateUnverified: (error: ApiError) => void;
    onInstallPackage: (pkg: string, version: string) => Promise<boolean>;
    onInstallNode: (version: string) => Promise<boolean>;
    /** A version the row's remove button chose (alpha.106): its removal opens here. */
    removeRequest?: { version: string; token: number } | null;
    onRemoveRequestTaken?: () => void;
    /** Opens another component's removal, named as a next action. */
    onRemoveComponent?: (id: string) => void;
    /** Opens the whole component's removal, every version. */
    onRemoveWhole?: () => void;
}) {
    const { t } = useI18n();
    const instances = service.instances ?? [];
    const tarball = isTarballRuntime(service);
    const blocked = (service.requires_missing?.length ?? 0) > 0;
    const nodeInstallHere = tarball && service.id === 'node' && !blocked;
    const [installing, setInstalling] = useState<string | null>(null);

    // Package-repo runtimes (php-fpm via Sury): installing an ADDITIONAL
    // version happens here too — before this, the version-pick dialog only
    // appeared while the service was NOT installed, so with 8.4 on the
    // machine there was no panel path to add 7.4 for a legacy site (caught
    // by the operator, 23 Jul). Same single-address rule as node: install
    // and remove live in the drawer.
    // Paket-depolu runtime'lar (Sury'li php-fpm): EK sürüm kurmak da burada —
    // bundan önce sürüm seçtiren pencere yalnız servis KURULU DEĞİLKEN
    // çıkıyordu; makinede 8.4 varken eski bir site için 7.4 eklemenin
    // panelden yolu yoktu (operatör yakaladı, 23 Tem). Node'la aynı
    // tek-adres kuralı: kur da kaldır da çekmecede.
    const [repo, setRepo] = useState<RepoInfo | null>(null);
    const [repoBusy, setRepoBusy] = useState(false);
    const repoError = repo?.error_code ? apiErrorText({ message: '', code: repo.error_code }, t, 'services.actionFailed') : '';
    const [repoAction, setRepoAction] = useState<'enable' | null>(null);
    useEffect(() => {
        if (tarball || service.kind !== 'runtime') return;
        fetch(`/api/v1/repo?service_id=${encodeURIComponent(service.id)}`)
            .then((r) => (r.ok ? r.json() : null))
            .then((d: RepoInfo | null) => setRepo(d && d.available ? d : null))
            .catch(() => {});
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [service.id]);

    const enableRepo = async () => {
        setRepoBusy(true);
        try {
            const res = await fetch('/api/v1/repo', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ service_id: service.id, action: 'enable' }),
            });
            if (!res.ok) {
                showToast('error', apiErrorText(await readApiError(res), t, 'services.actionFailed'));
                return;
            }
            const d: RepoInfo = await res.json();
            setRepo(d);
        } catch {
            showToast('error', t('services.actionFailed'));
        } finally {
            setRepoBusy(false);
        }
    };

    const installPackage = async (pkg: string, version: string) => {
        if (installing) return;
        setInstalling(pkg);
        try {
            await onInstallPackage(pkg, version);
        } finally {
            setInstalling(null);
        }
    };

    // "php8.2-fpm" → "8.2". / "php8.2-fpm" → "8.2".
    const pkgVersion = (pkg: string) => /([0-9]+\.[0-9]+)/.exec(pkg)?.[1] ?? '';
    // Every PHP line below 8.2 is past upstream security support in mid-2026.
    // A static threshold is honest enough until a maintained EOL table exists
    // (B4 note); the point is that a legacy line is never offered unlabeled.
    // 8.2 altındaki her PHP hattı 2026 ortasında güvenlik desteğinin dışında.
    // Bakımlı bir EOL tablosu gelene dek (B4 notu) sabit eşik yeterince
    // dürüst; mesele eski hattın asla etiketsiz sunulmaması.
    const isEOL = (version: string) => {
        const m = /^([0-9]+)\.([0-9]+)$/.exec(version);
        if (!m) return false;
        const [maj, min] = [Number(m[1]), Number(m[2])];
        return maj < 8 || (maj === 8 && min < 2);
    };

    // Named LTS options from the official index — the free-text semver box
    // died with B3d: an operator picks "Node 24 (LTS)" the way they pick
    // "PHP 8.3", they do not transcribe version numbers.
    // Resmi dizinden adlandırılmış LTS seçenekleri — serbest semver kutusu
    // B3d ile öldü: operatör "PHP 8.3" seçer gibi "Node 24 (LTS)" seçer,
    // sürüm numarası kopyalamaz.
    const [lts, setLts] = useState<{ version: string; name: string }[] | null>(null);
    const [ltsFailed, setLtsFailed] = useState(false);
    const loadLts = () => {
        setLtsFailed(false);
        setLts(null);
        fetch('/api/v1/runtimes/node/lts')
            .then((r) => (r.ok ? r.json() : Promise.reject()))
            .then((d) => setLts(d?.releases ?? []))
            .catch(() => setLtsFailed(true));
    };
    useEffect(() => {
        if (nodeInstallHere) loadLts();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [nodeInstallHere]);

    const installVersion = async (version: string) => {
        if (installing) return;
        setInstalling(version);
        try {
            await onInstallNode(version);
        } finally {
            setInstalling(null);
        }
    };

    // Per-version removal (B3d) — the reason the drawer may offer delete at
    // all: the panel refuses with the blocking-site list while anything runs
    // on the version, and that refusal renders right here in the confirm
    // dialog. php versions go through the package pick; node versions
    // through their own endpoint. Unmanaged ("system") rows never get one.
    // Sürüm başına kaldırma (B3d) — çekmecenin silme sunabilmesinin sebebi:
    // sürümün üstünde bir şey koşarken panel, engelleyen-site listesiyle
    // reddeder ve o ret tam burada, onay penceresinde çizilir. php sürümleri
    // paket seçiminden, node sürümleri kendi ucundan gider. Yönetilmeyen
    // ("sistem") satıra silme verilmez.
    const [removeTarget, setRemoveTarget] = useState<ServiceInstance | null>(null);
    const [removeError, setRemoveError] = useState<ApiError | null>(null);
    const [removing, setRemoving] = useState(false);
    // alpha.104: who uses each PHP version (sites, webmail, phpMyAdmin,
    // phpPgAdmin), read once when the list opens and after a change; the
    // removal dialog reads what the removal would take, and offers to move the
    // tools first.
    // Her PHP sürümünü kimin kullandığı; kaldırma penceresi araçları önce taşımayı sunar.
    const phpVersions = service.id === 'php-fpm' && service.is_installed === true;
    const [phpTools, setPhpTools] = useState<PHPToolsInfo | null>(null);
    const [phpToolsToken, setPhpToolsToken] = useState(0);
    const instanceVersions = instances.map((inst) => inst.version).join(',');
    useEffect(() => {
        if (!phpVersions) return;
        const controller = typeof AbortController === 'undefined' ? null : new AbortController();
        void readPHPTools(controller?.signal).then((info) => { if (!controller?.signal.aborted) setPhpTools(info); });
        return () => controller?.abort();
    }, [phpVersions, instanceVersions, phpToolsToken]);
    const versionImpact = useRemovalImpact(service.id, Boolean(removeTarget?.unit) && phpVersions, removeTarget?.unit ?? '');
    const [moveTo, setMoveTo] = useState('');
    const [moveFailure, setMoveFailure] = useState<PHPToolMoveFailure | null>(null);
    const toolName = (tool: string) => componentName({ id: PHP_TOOL_SERVICE_ID[tool as keyof typeof PHP_TOOL_SERVICE_ID] ?? tool, name: tool }, t);
    const removeVersion = async (inst: ServiceInstance, moveToolsTo = '') => {
        setRemoving(true);
        setRemoveError(null);
        setMoveFailure(null);
        try {
            const res = inst.unit
                ? await fetch('/api/v1/service/uninstall', {
                      method: 'POST',
                      headers: { 'Content-Type': 'application/json' },
                      body: JSON.stringify({ service_id: service.id, package: inst.unit, ...(moveToolsTo ? { move_tools_to: moveToolsTo } : {}) }),
                  })
                : await fetch(`/api/v1/runtimes/node/${inst.version}`, { method: 'DELETE' });
            if (!res.ok) {
                const raw = await answerBody(res);
                const error = await readApiError(res);
                // A move that did not complete: say what moved, what was put
                // back and that the version stays (alpha.104).
                // Tamamlanmayan taşıma: neyin taşındığını, neyin geri konduğunu söyle.
                if (error.code === 'PHP_TOOL_MOVE_FAILED') {
                    setMoveFailure(decodePHPToolMoveFailure(raw));
                    setPhpToolsToken((n) => n + 1);
                    versionImpact.retry();
                    await onVersionInstalled();
                    return;
                }
                // The tools moved, then the removal was refused or its answer
                // was lost (alpha.104 review): say what moved, then the
                // refusal, and read everything again.
                // Araçlar taşındı, sonra kaldırma reddedildi ya da yanıtı kayboldu.
                const afterMoves = decodePHPToolMoveFailure(raw);
                if (afterMoves && afterMoves.moved.length > 0) {
                    const movedTo = typeof (raw as { moved_to?: unknown } | null)?.moved_to === 'string' ? String((raw as { moved_to: string }).moved_to) : moveToolsTo;
                    setMoveFailure({ ...afterMoves, tool: '', version: afterMoves.version || movedTo, error: '' });
                    // After moves the refusal never says "Nothing was removed":
                    // the tools moved. Its after-moves wording leads with the version.
                    // Taşımalardan sonra ret "Hiçbir şey kaldırılmadı" demez.
                    setRemoveError(error.code === 'REMOVAL_UNKNOWN_AFTER_MOVES' ? null : { ...error, code: 'afterMoves.' + (AFTER_MOVES_CODES.has(error.code ?? '') ? error.code : 'REMOVAL_REFUSED') });
                    setPhpToolsToken((n) => n + 1);
                    versionImpact.retry();
                    await onVersionInstalled();
                    return;
                }
                if (error.code === 'SERVICE_STATE_REFRESH_FAILED') {
                    setRemoveTarget(null);
                    onStateUnverified(error);
                    return;
                }
                // These responses carry a verified post-mutation snapshot.
                // Warn, close the stale modal and reload instead of pretending
                // the original row is still authoritative.
                // Bu yanıtlar değişiklik sonrası doğrulanmış snapshot taşır.
                // Eski satırı yetkili sanmak yerine uyar, modalı kapat ve yeniden yükle.
                if (
                    error.code === 'FIREWALL_SYNC_FAILED' ||
                    error.code === 'MAIL_FILTER_SYNC_FAILED' ||
                    error.code === 'SERVICE_UNINSTALL_PARTIAL'
                ) {
                    setRemoveTarget(null);
                    showToast('error', apiErrorText(error, t, 'services.actionFailed'));
                    await onVersionInstalled();
                    return;
                }
                setRemoveError(error);
                return;
            }
            setRemoveTarget(null);
            const done = await answerBody(res) as { moved?: unknown; moved_to?: unknown } | null;
            const moved = Array.isArray(done?.moved) ? (done?.moved as unknown[]).filter((tool): tool is string => typeof tool === 'string') : [];
            showToast('success', moved.length > 0
                ? t('services.version.movedAndRemoved', { tools: moved.map(toolName).join(', '), target: String(done?.moved_to ?? moveToolsTo), version: inst.version })
                : t('services.versionRemoved', { version: inst.version }));
            setPhpToolsToken((n) => n + 1);
            // The endpoint mutation is not enough to trust the old row. Wait
            // for the parent scan and keep its safety lock when verification
            // fails.
            // Uç nokta mutasyonu eski satıra güvenmek için yeterli değildir.
            // Üst bileşenin taramasını bekle; doğrulama başarısızsa güvenlik
            // kilidini koru.
            if (!(await onVersionInstalled())) return;
        } catch {
            onStateUnverified({ message: t('services.actionFailed') });
        } finally {
            setRemoving(false);
        }
    };
    const canRemove = (inst: ServiceInstance) =>
        inst.managed && (inst.unit ? service.kind === 'runtime' : service.id === 'node');
    // The row's "which version?" answer continues into this version's own
    // removal, with its move-then-remove (alpha.106).
    // Satırdaki "hangi sürüm?" yanıtı bu sürümün kendi kaldırmasına geçer.
    useEffect(() => {
        if (!removeRequest) return;
        const inst = instances.find((candidate) => candidate.version === removeRequest.version && canRemove(candidate));
        onRemoveRequestTaken?.();
        if (!inst) return;
        setRemoveError(null);
        setMoveFailure(null);
        setRemoveTarget(inst);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [removeRequest?.token]);

    return (
        <fieldset
            disabled={mutationDisabled}
            aria-busy={mutationDisabled}
            className="relative mt-1 w-full rounded-lg border border-border bg-surface-2/40 px-4 py-3"
        >
            <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-fg-subtle">
                {t('services.instancesTitle')}
            </div>
            {instances.length === 0 ? (
                <div className="py-1 text-sm text-fg-subtle">{t('services.instancesEmpty')}</div>
            ) : (
                <ul className="divide-y divide-border/60">
                    {instances.map((inst) => {
                        const instRunning = isServiceRunning(inst.status);
                        const instBusy = busy === `${service.id}:${inst.unit}`;
                        return (
                            <li key={`${inst.version}:${inst.unit ?? inst.path ?? ''}`} className="flex flex-wrap items-center gap-x-4 gap-y-1 py-2">
                                <span className="w-16 shrink-0 font-mono text-sm font-medium text-fg">{inst.version || '—'}</span>
                                {phpVersions && phpTools && <VersionUsage info={phpTools} version={inst.version} toolName={toolName} />}
                                {!inst.managed && (
                                    <span
                                        title={t('services.systemRuntimeHint')}
                                        className="rounded bg-surface-2 px-1.5 py-0.5 text-xs font-semibold uppercase tracking-wide text-fg-subtle"
                                    >
                                        {t('services.systemRuntime')}
                                    </span>
                                )}
                                {inst.unit ? (
                                    <span className="inline-flex w-28 items-center gap-1.5 text-sm text-fg-muted">
                                        <StatusDot tone={instRunning ? 'success' : 'danger'} />
                                        {instRunning ? t('services.running') : t('services.stopped')}
                                    </span>
                                ) : (
                                    /* No unit → nothing to run; "installed" is the whole
                                       truth. / Unit yok → koşacak şey yok; "kurulu" tam
                                       gerçektir. */
                                    <span className="w-28 text-sm text-fg-subtle">{t('services.installedLabel')}</span>
                                )}
                                {inst.unit && <span className="hidden font-mono text-xs text-fg-subtle md:inline">{inst.unit}</span>}
                                {(inst.size_bytes ?? 0) > 0 && (
                                    <span className="text-xs text-fg-subtle">{fmtBytes(inst.size_bytes as number)}</span>
                                )}
                                {(inst.unit && inst.managed) || canRemove(inst) ? (
                                    <span className="ml-auto flex items-center gap-1">
                                        {inst.unit && inst.managed && (instRunning ? (
                                            <>
                                                <ActionIcon
                                                    title={t('services.restart')}
                                                    onClick={() => onInstanceAction(service.id, inst.unit as string, 'restart')}
                                                    disabled={instBusy}
                                                    tone="warning"
                                                >
                                                    <RotateCw className="h-4 w-4" />
                                                </ActionIcon>
                                                <ActionIcon
                                                    title={t('services.stop')}
                                                    onClick={() => onInstanceAction(service.id, inst.unit as string, 'stop')}
                                                    disabled={instBusy}
                                                    tone="danger"
                                                >
                                                    <StopCircle className="h-4 w-4" aria-hidden="true" />
                                                </ActionIcon>
                                            </>
                                        ) : (
                                            <ActionIcon
                                                title={t('services.start')}
                                                onClick={() => onInstanceAction(service.id, inst.unit as string, 'start')}
                                                disabled={instBusy}
                                                tone="success"
                                            >
                                                <Play className="h-4 w-4" aria-hidden="true" />
                                            </ActionIcon>
                                        ))}
                                        {canRemove(inst) && (
                                            <ActionIcon
                                                title={t('services.removeVersion')}
                                                onClick={() => { setRemoveError(null); setMoveFailure(null); setRemoveTarget(inst); }}
                                                disabled={removing}
                                                tone="danger"
                                            >
                                                <Trash2 className="h-4 w-4" />
                                            </ActionIcon>
                                        )}
                                    </span>
                                ) : null}
                            </li>
                        );
                    })}
                </ul>
            )}
            {/* Additional versions for a repo-backed runtime (php via Sury).
                Not-yet-installed lines render as install buttons; EOL lines
                are labeled, never hidden — a legacy site is a legitimate
                reason, an unlabeled trap is not.
                Depo-destekli runtime'da ek sürümler (Sury'li php). Kurulu
                olmayan hatlar kurulum düğmesi olur; EOL hatlar etiketlenir,
                asla gizlenmez — eski site meşru sebeptir, etiketsiz tuzak
                değildir. */}
            {!tarball && repo && (
                <div className="mt-2 border-t border-border/60 pt-3">
                    {!repo.enabled ? (
                        <div className="flex flex-wrap items-center gap-3">
                            <span className={`text-xs ${repoError ? 'text-danger' : 'text-fg-subtle'}`}>
                                {repoError || t('services.repo.note')}
                            </span>
                            <Button variant="secondary" onClick={() => setRepoAction('enable')} disabled={repoBusy || Boolean(repo.error_code && !repo.repairable)} loading={repoBusy}>
                                {repoBusy ? t('services.repo.enabling') : t('services.repo.enable')}
                            </Button>
                        </div>
                    ) : (
                        <>
                            <div className="flex flex-wrap items-center gap-2">
                                {(repo.packages ?? []).map((pkg) => {
                                    const v = pkgVersion(pkg);
                                    const already = instances.some((i) => i.managed && i.version === v);
                                    if (!v || already) return null;
                                    return (
                                        <button
                                            key={pkg}
                                            onClick={() => installPackage(pkg, v)}
                                            disabled={installing !== null}
                                            title={isEOL(v) ? t('services.eolHint') : pkg}
                                            className="inline-flex items-center gap-1.5 rounded-lg border border-border-strong bg-surface px-3 py-1.5 text-xs font-semibold text-fg transition-colors hover:bg-surface-2 disabled:opacity-50"
                                        >
                                            <DownloadCloud className="h-3.5 w-3.5" />
                                            {installing === pkg ? t('services.installing') : `${componentName(service, t)} ${v}`}
                                            {isEOL(v) && (
                                                <span className="rounded bg-warning/15 px-1 py-0.5 text-xs font-bold uppercase text-warning">
                                                    {t('services.eolBadge')}
                                                </span>
                                            )}
                                        </button>
                                    );
                                })}
                            </div>
                            <div className="mt-1.5 text-xs text-fg-subtle">{t('services.phpVersionsHint')}</div>
                        </>
                    )}
                </div>
            )}
            {/* The install endpoint is node-specific today; when a second
                tarball runtime arrives the endpoint generalizes and this id
                check dissolves into it. Posting python versions to the node
                endpoint is the bug this line prevents.
                Kurulum ucu bugün node'a özgü; ikinci bir tarball runtime
                gelince uç genelleşir ve bu id denetimi onun içinde erir.
                Node ucuna python sürümü göndermek, bu satırın önlediği hatadır. */}
            {tarball && service.id === 'node' && (
                <div className="mt-2 border-t border-border/60 pt-3">
                    {blocked ? (
                        /* The same declarative gate as the row's Install button:
                           requirements first. / Satırdaki Kur ile aynı bildirimsel
                           kapı: önce gereksinimler. */
                        <div className="text-sm text-fg-subtle">
                            {t('services.requiresLabel', {
                                names: (service.requires_missing ?? [])
                                    .map((tok) => (REQ_ROLE_KEYS[tok] ? t(REQ_ROLE_KEYS[tok] as Parameters<typeof t>[0]) : tok))
                                    .join(', '),
                            })}
                        </div>
                    ) : (
                        <>
                            {lts === null && !ltsFailed && (
                                <div className="text-sm text-fg-subtle">{t('services.ltsLoading')}</div>
                            )}
                            {ltsFailed && (
                                <div className="flex flex-wrap items-center gap-3 text-sm text-fg-subtle">
                                    {t('services.ltsFailed')}
                                    <Button variant="secondary" onClick={loadLts}>{t('services.ltsRetry')}</Button>
                                </div>
                            )}
                            {lts !== null && (
                                <div className="flex flex-wrap items-center gap-2">
                                    {lts.map((rel) => {
                                        const already = instances.some((i) => i.managed && i.version === rel.version);
                                        const major = rel.version.split('.')[0];
                                        return (
                                            <button
                                                key={rel.version}
                                                onClick={() => installVersion(rel.version)}
                                                disabled={already || installing !== null}
                                                className="inline-flex items-center gap-1.5 rounded-lg border border-border-strong bg-surface px-3 py-1.5 text-xs font-semibold text-fg transition-colors hover:bg-surface-2 disabled:opacity-50"
                                            >
                                                <DownloadCloud className="h-3.5 w-3.5" />
                                                {installing === rel.version
                                                    ? t('services.installing')
                                                    : already
                                                      ? t('services.ltsInstalled', { major })
                                                      : `Node ${major} LTS · ${rel.version}`}
                                            </button>
                                        );
                                    })}
                                </div>
                            )}
                            <div className="mt-1.5 text-xs text-fg-subtle">{t('services.nodeInstallHint')}</div>
                        </>
                    )}
                </div>
            )}
            {repoAction && repo && (
                <RepositoryActionConfirmationDialog
                    action={repoAction}
                    repositoryName={repo.name || componentName(service, t)}
                    confirmDisabled={repoBusy}
                    onCancel={() => setRepoAction(null)}
                    onConfirm={() => {
                        setRepoAction(null);
                        void enableRepo();
                    }}
                />
            )}
                {/* No backdrop dismissal on a DESTRUCTIVE dialog. The operator
                    pressed Uninstall on BIND, clicked next to the box, and the
                    dialog vanished without a trace — which reads exactly like
                    "I said remove and it did not remove" (24 Jul; the audit log
                    later showed the request had never been sent). A silent
                    cancel is indistinguishable from a broken button, so leaving
                    requires the explicit Cancel. The install dialog keeps its
                    backdrop dismissal: dropping an install prompt costs nothing.
                    Yıkıcı diyalogda arka plana tıklayarak kapatma YOK. Operatör
                    BIND'de Kaldır'a bastı, kutunun yanına tıkladı ve diyalog iz
                    bırakmadan kayboldu — bu, "kaldır dedim kaldırmadı" diye
                    okunur (24 Tem; denetim kaydı isteğin hiç gönderilmediğini
                    sonradan gösterdi). Sessiz vazgeçme, bozuk düğmeden ayırt
                    edilemez; çıkmak için açıkça Vazgeç gerekir. Kurulum
                    diyalogu arka plan kapatmasını korur: kurulum istemini
                    düşürmenin bedeli yok. */}
            {removeTarget && (
                <Dialog
                    id="service-remove-version"
                    tone="danger"
                    icon={Trash2}
                    dismissible={false}
                    busy={removing}
                    title={t('services.removeVersionTitle', { name: componentName(service, t), version: removeTarget.version })}
                    actions={(() => {
                        const known = removeTarget.unit && phpVersions && versionImpact.read.state === 'known' ? versionImpact.read.impact : null;
                        const sim = known?.simulation ?? null;
                        const sitesBlock = Boolean(known?.hosted?.blocks_removal);
                        const collateral = sim?.state === 'checked' && sim.collateral.length > 0;
                        const toolsBound = (sim?.bound_tools.length ?? 0) > 0;
                        const targets = otherVersions(phpTools, removeTarget.version);
                        const target = targets.includes(moveTo) ? moveTo : targets[0] ?? '';
                        // Offered only when the package manager answered and
                        // takes nothing else: otherwise the tools would move
                        // and the version stay (alpha.104 review).
                        // Yalnız paket yöneticisi yanıtladıysa ve başka bir şey almıyorsa.
                        const offerMove = toolsBound && sim?.state === 'checked' && !sitesBlock && !collateral && target !== '';
                        const refused = (Boolean(sim?.refuses) || sitesBlock) && !offerMove;
                        if (refused) return <RefusedActions onClose={() => setRemoveTarget(null)} busy={removing} />;
                        return (
                            <>
                                <Button variant="secondary" onClick={() => setRemoveTarget(null)} disabled={removing}>
                                    {t('common.cancel')}
                                </Button>
                                {offerMove ? (
                                    <Button variant="danger" onClick={() => removeVersion(removeTarget, target)} disabled={removing} loading={removing} icon={Trash2} data-version-move-remove="">
                                        {removing ? t('services.version.moving', { version: target }) : t('services.version.moveButton', { version: target })}
                                    </Button>
                                ) : (
                                    <Button variant="danger" onClick={() => removeVersion(removeTarget)} disabled={removing} loading={removing} icon={Trash2}>
                                        {removing ? t('services.uninstalling') : t('services.removeVersion')}
                                    </Button>
                                )}
                            </>
                        );
                    })()}
                >
                    {/* What a failed move did comes first, where it is seen.
                        Başarısız taşımanın yaptığı önce, görüldüğü yerde. */}
                    {moveFailure && <MoveFailureNote failure={moveFailure} removed={removeTarget.version} toolName={toolName} />}
                    {/* After moves, the refusal stands with them, where it is seen
                        on a phone too. / Taşımalardan sonra ret onların yanında. */}
                    {moveFailure && <ErrorBanner error={removeError} className="mb-3" />}
                    {removeTarget.unit && phpVersions && (
                        <VersionRemovalImpact
                            name={`${componentName(service, t)} ${removeTarget.version}`}
                            whole={componentName(service, t)}
                            serviceId={service.id}
                            version={removeTarget.version}
                            read={versionImpact.read}
                            onRetry={versionImpact.retry}
                            targets={otherVersions(phpTools, removeTarget.version)}
                            versions={phpTools === null ? 'checking' : phpTools.state === 'read' ? 'read' : 'unknown'}
                            onRetryVersions={() => setPhpToolsToken((n) => n + 1)}
                            moveTo={moveTo}
                            onMoveTo={setMoveTo}
                            moving={removing}
                            nameOf={(id) => componentName({ id, name: id }, t)}
                            onRemoveComponent={onRemoveComponent ? (id) => { setRemoveTarget(null); onRemoveComponent(id); } : undefined}
                            onRemoveWhole={onRemoveWhole ? () => { setRemoveTarget(null); onRemoveWhole(); } : undefined}
                        />
                    )}
                    {/* The refusal's evidence — the blocking sites — lands
                        here, in the confirm dialog (B3d).
                        Retin kanıtı — engelleyen siteler — buraya, onay
                        penceresine düşer (B3d). */}
                    {!moveFailure && <ErrorBanner error={removeError} />}
                </Dialog>
            )}
        </fieldset>
    );
}

// Refusals with their own after-moves wording (alpha.104 review).
const AFTER_MOVES_CODES: ReadonlySet<string> = new Set(['REMOVAL_PLAN_CHANGED', 'REMOVAL_WOULD_REMOVE_OTHERS', 'REMOVAL_SIMULATION_FAILED', 'REMOVAL_TOOLS_BOUND']);

// The installed PHP versions the tools can move to: every other installed
// version, newest first.
// Araçların taşınabileceği kurulu PHP sürümleri.
function otherVersions(info: PHPToolsInfo | null, removed: string): string[] {
    if (!info || info.state !== 'read') return [];
    return info.versions.map((usage) => usage.version).filter((version) => version !== removed);
}

// "Used by 3 sites, Webmail, phpMyAdmin" beside a PHP version (alpha.104, B).
// Bir PHP sürümünün yanında onu kimin kullandığı.
function VersionUsage({ info, version, toolName }: { info: PHPToolsInfo; version: string; toolName: (tool: string) => string }) {
    const { t } = useI18n();
    if (info.state !== 'read') return null;
    const usage = info.versions.find((item) => item.version === version);
    if (!usage) return null;
    const who: string[] = [];
    if (usage.sites > 0) who.push(t(usage.sites === 1 ? 'services.version.oneSite' : 'services.version.sites', { count: usage.sites }));
    for (const tool of usage.tools) who.push(toolName(tool));
    return (
        <span className="text-xs text-fg-muted" data-version-usage={version} title={usage.site_names.join(', ') || undefined}>
            {who.length > 0 ? t('services.version.usedBy', { who: who.join(', ') }) : t('services.version.unused')}
        </span>
    );
}

// What removing one PHP version takes, before the button (alpha.104, C):
// the sites on it (moved in each site's settings), the tools on it (moved
// here, verified, before the removal), and the package manager's answer.
// Tek PHP sürümünü kaldırmanın aldıkları, düğmeden önce.
function VersionRemovalImpact({ name, whole, serviceId, version, read, onRetry, targets, versions, onRetryVersions, moveTo, onMoveTo, moving, nameOf, onRemoveComponent, onRemoveWhole }: {
    name: string;
    /** The component's own name, for "remove it with all its versions". */
    whole: string;
    serviceId: string;
    version: string;
    read: RemovalRead;
    onRetry: () => void;
    targets: string[];
    /** Whether the installed versions were read: "no other version" is said
     *  only when they were. / Kurulu sürümler okundu mu. */
    versions: 'checking' | 'read' | 'unknown';
    onRetryVersions: () => void;
    moveTo: string;
    onMoveTo: (version: string) => void;
    moving: boolean;
    nameOf: (id: string) => string;
    onRemoveComponent?: (id: string) => void;
    onRemoveWhole?: () => void;
}) {
    const { t } = useI18n();
    if (read.state === 'checking') return <div className="mb-3" data-removal-impact="checking"><Checking label={t('services.removal.checkingVersion', { name })} /></div>;
    if (read.state === 'refused') {
        return (
            <div className="mb-3" data-removal-impact="refused">
                <RemovalAnswer verdict="blocked" subject={name} causes={[]} steps={[]} notes={[]}><ErrorBanner error={read.error} /></RemovalAnswer>
            </div>
        );
    }
    if (read.state === 'unknown') return <div className="mb-3" data-removal-impact="unknown"><CouldNotCheck text={t('services.removal.unknownVersion', { name })} onRetry={onRetry} /></div>;
    const { impact } = read;
    const sim = impact.simulation;
    const hosted = impact.hosted && impact.hosted.count > 0 ? impact.hosted : null;
    const shown = hosted ? hosted.names.slice(0, REMOVAL_IMPACT_NAMES_SHOWN) : [];
    const more = hosted ? hosted.count - shown.length : 0;
    const sitesBlock = Boolean(hosted?.blocks_removal);
    // The move is offered only when the tools are the one thing in the way:
    // with sites on the version or packages the removal would also take,
    // moving the tools would not make the removal possible.
    // Taşıma yalnız araçlar tek engel olduğunda sunulur.
    const toolsBound = (sim?.bound_tools.length ?? 0) > 0 && !impact.hosted?.blocks_removal
        && sim?.state === 'checked' && sim.collateral.length === 0;
    const target = targets.includes(moveTo) ? moveTo : targets[0] ?? '';
    const findings = sim ? simulationFindings(t, { name, whole, serviceId, simulation: sim, nameOf, version, onRemoveComponent, onRemoveWhole }) : NO_FINDINGS;
    const causes: RemovalItem[] = [];
    const steps: RemovalItem[] = [];
    if (hosted) {
        causes.push({ key: 'sites', line: 'sites', text: t('services.version.sitesOn', { count: hosted.count, version, names: shown.join(', ') + (more > 0 ? ' ' + t('services.removal.more', { n: more }) : '') }) });
        if (sitesBlock) steps.push({ key: 'sites', text: t('services.version.sitesMove'), to: '/domains', linkText: t('services.version.sitesLink') });
    }
    causes.push(...findings.causes);
    steps.push(...findings.steps);
    // Tools on the version with another version to move to: removable after
    // the move. With no other version installed it cannot go now.
    // Araçlar taşınabiliyorsa taşımadan sonra kaldırılabilir.
    const moveOpen = toolsBound && !(versions === 'read' && targets.length === 0);
    if (moveOpen) steps.push({ key: 'move', text: t('services.removal.next.moveTools') });
    const verdict = moveOpen ? 'move' : (sim?.refuses || sitesBlock) ? 'blocked' : 'allowed';
    return (
        <div className="mb-3" data-removal-impact="known">
            <RemovalAnswer
                verdict={verdict}
                subject={name}
                causes={causes}
                steps={steps}
                notes={findings.notes}
                details={(
                    <>
                        <p className="text-fg-muted">{t('services.removeVersionWarn')}</p>
                        {findings.details.map((item) => <p key={item.key} className="text-fg-muted"><RemovalItemText item={item} /></p>)}
                        <SimulatedList simulation={sim} refused={verdict !== 'allowed'} />
                    </>
                )}
            >
                {toolsBound && (
                    versions === 'checking' ? (
                        <Checking label={t('services.version.versionsChecking')} />
                    ) : versions === 'unknown' ? (
                        <div data-version-move="unknown"><CouldNotCheck text={t('services.version.versionsUnknown')} onRetry={onRetryVersions} /></div>
                    ) : targets.length > 0 ? (
                        <label className="flex flex-wrap items-center gap-2 text-fg" data-version-move="">
                            <span>{t('services.version.moveLabel')}</span>
                            <select
                                value={target}
                                onChange={(event) => onMoveTo(event.target.value)}
                                disabled={moving}
                                className="rounded-lg border border-border-strong bg-surface px-2 py-1 font-mono text-sm text-fg max-sm:min-h-11"
                            >
                                {targets.map((option) => <option key={option} value={option}>PHP {option}</option>)}
                            </select>
                            <span className="text-xs text-fg-muted">{t('services.version.moveHint')}</span>
                        </label>
                    ) : (
                        <p className="text-fg-muted" data-version-move="none">{t('services.version.moveNone')}</p>
                    )
                )}
                {moving && toolsBound && (
                    <div className="flex items-start gap-2 text-sm text-fg-muted" data-version-moving="">
                        <span className="mt-0.5 shrink-0"><Spinner size="xs" label={t('services.version.movingWait', { version: target })} /></span>
                        <span aria-hidden="true">{t('services.version.movingWait', { version: target })}</span>
                    </div>
                )}
            </RemovalAnswer>
        </div>
    );
}

// A move before a removal that did not complete, said exactly (alpha.104).
// Tamamlanmayan taşıma, tam olarak.
function MoveFailureNote({ failure, removed, toolName }: { failure: PHPToolMoveFailure; removed: string; toolName: (tool: string) => string }) {
    const { t } = useI18n();
    const parts: string[] = [];
    if (failure.moved.length > 0) parts.push(t('services.version.moveDone', { tools: failure.moved.map(toolName).join(', '), version: failure.version }));
    const reason = phpToolCauseText(t, failure.cause, failure.detail || failure.error);
    if (failure.tool) {
        parts.push(failure.restored
            ? t('services.version.moveRestored', { tool: toolName(failure.tool), version: failure.version, previous: failure.previous || removed, reason })
            : t('services.version.moveNotRestored', { tool: toolName(failure.tool), version: failure.version, reason: failure.cause && failure.error ? reason + ' — ' + t('component.phpTool.cause.other', { detail: failure.error }) : reason }));
    } else if (failure.error) {
        parts.push(failure.error);
    }
    if (failure.not_attempted.length > 0) parts.push(t('services.version.moveNotAttempted', { tools: failure.not_attempted.map(toolName).join(', ') }));
    // A lost removal answer after the moves: whether the version went is
    // not known, so "was not removed" is not said.
    // Taşımalardan sonra kaybolan yanıt: sürümün gidip gitmediği bilinmez.
    parts.push(failure.code === 'REMOVAL_UNKNOWN_AFTER_MOVES'
        ? t('services.version.removalUnknown', { version: removed })
        : t('services.version.moveKept', { version: removed }));
    return (
        <div className="mb-3 rounded-lg border border-danger/30 bg-danger/5 p-3 text-sm text-fg" data-version-move-failure={failure.tool}>
            {parts.map((part, index) => <p key={index} className={index > 0 ? 'mt-1' : undefined}><KeepWords text={part} /></p>)}
        </div>
    );
}

function RepositoryActionConfirmationDialog({
    action,
    repositoryName,
    confirmDisabled,
    onCancel,
    onConfirm,
}: {
    action: 'enable' | 'disable';
    repositoryName: string;
    confirmDisabled: boolean;
    onCancel: () => void;
    onConfirm: () => void;
}) {
    const { t } = useI18n();
    const key = (suffix: 'title' | 'description' | 'button') => (
        ('services.repo.confirm.' + action + '.' + suffix) as Parameters<typeof t>[0]
    );
    // Opened from inside the install dialogue, so it stacks above it and takes
    // Escape while it is there.
    // Kurulum diyalogunun icinden acilir; onun ustunde durur ve oradayken
    // Escape'i o alir.
    return (
        <Dialog
            id='repository-action-confirm'
            stacked
            title={t(key('title'), { name: repositoryName })}
            description={t(key('description'), { name: repositoryName })}
            onDismiss={onCancel}
            actions={
                <>
                    <Button variant='secondary' autoFocus onClick={onCancel}>
                        {t('common.cancel')}
                    </Button>
                    <Button
                        variant={action === 'disable' ? 'danger' : 'primary'}
                        icon={Layers}
                        disabled={confirmDisabled}
                        onClick={onConfirm}
                    >
                        {t(key('button'))}
                    </Button>
                </>
            }
        />
    );
}

// What the webmail cleanup removes, as the Agent names it: the Roundcube tree
// (webmailBaseDir), the copies an interrupted install or removal leaves beside
// it (roundcubeStageName / roundcubeRetirementName), the Nginx entry
// (webmailConfPath) and the inactive webmail socket (WebmailSocketPath).
// Web postası temizliğinin sildikleri, Agent'ın adlandırdığı gibi.
const WEBMAIL_CLEANUP_PATHS: { paths: string[]; key: Parameters<ReturnType<typeof useI18n>['t']>[0] }[] = [
    { paths: ['/var/lib/celikpanel-webmail'], key: 'services.cleanupWebmailPath.tree' },
    { paths: ['/var/lib/.celikpanel-webmail.stage', '/var/lib/.celikpanel-webmail.retired*', '/var/lib/..celikpanel-webmail.stage.retired*'], key: 'services.cleanupWebmailPath.copies' },
    { paths: ['/etc/nginx/conf.d/celikpanel-webmail.conf'], key: 'services.cleanupWebmailPath.nginx' },
    { paths: ['/run/celikpanel-webmail.sock'], key: 'services.cleanupWebmailPath.socket' },
];

// What a removal affects on this server, as the Panel answers
// GET /api/v1/service/removal-impact (alpha.100, owner decision 2026-10-22).
// The Panel reads only what it already holds — the catalogue, the stored
// component record and its database — and reports the uninstall path's own
// conditions. The dialog reads it once when it opens: a read that fails or
// takes longer than its bound says so and the removal stays allowed (the
// Panel's own refusals still apply when Remove is pressed).
// Kaldırmanın bu sunucuda neyi etkilediği, Panel'in yanıtıyla. Pencere bir
// kez okur; okunamazsa söyler ve kaldırma yine izinlidir.
export type RemovalHostedKind = 'sites' | 'php_sites' | 'node_sites' | 'databases' | 'mail_addresses' | 'mail_domains' | 'mailboxes'
    | 'certificates' | 'firewall' | 'unrecorded' | 'vpn_peers' | 'ftp_accounts';
export interface RemovalImpact {
    removal: 'packages' | 'webmail_tree' | 'unsupported';
    has_units: boolean;
    setup_required: boolean;
    package_family: string;
    packages: string[];
    all_versions: boolean;
    firewall_ports: string[];
    mail_filter_rewired: boolean;
    database_tools_reconfigured: boolean;
    required_by: string[];
    bundles: { id: string; name_code: string }[];
    hosted: {
        kind: RemovalHostedKind; count: number; names: string[]; blocks_removal: boolean;
        firewall?: { known: boolean; enabled: boolean; open_ports: number };
    } | null;
    /** alpha.104: the package manager's own answer for this exact removal. */
    simulation: RemovalSimulation | null;
    /** alpha.104: one version of a runtime ("8.4"), or ''. */
    version: string;
}

// What the package manager says the removal would take (alpha.104, after the
// Frankfurt removal of PHP-FPM 8.4 that also purged phpMyAdmin): exactly
// these packages; anything it would take that the removal does not name
// refuses the removal; the PHP tools running on the PHP being removed.
// Paket yöneticisinin bu kaldırma için yanıtı.
export interface RemovalSimulation {
    state: 'checked' | 'busy' | 'unknown' | 'unsupported' | 'not_installed';
    remove: string[];
    collateral: { package: string; reason: 'component' | 'reverse_dependency' | 'would_install'; component: string }[];
    bound_tools: string[];
    default_runtime: boolean;
    refuses: boolean;
    /** alpha.107 (D-038 §8): a database engine is removed, not purged; where its data stays. */
    keeps_data?: boolean;
    data_paths?: string[];
}
const SIMULATION_STATES: ReadonlySet<string> = new Set(['checked', 'busy', 'unknown', 'unsupported', 'not_installed']);
const COLLATERAL_REASONS: ReadonlySet<string> = new Set(['component', 'reverse_dependency', 'would_install']);
// The Agent's PHP tool names and their components.
// Agent'ın PHP aracı adları ve bileşenleri.
export const PHP_TOOL_SERVICE: Record<string, string> = { webmail: 'roundcube', phpmyadmin: 'phpmyadmin', phppgadmin: 'phppgadmin' };

function decodeRemovalSimulation(value: unknown): RemovalSimulation | null {
    if (value === undefined || value === null) return null;
    const unknown: RemovalSimulation = { state: 'unknown', remove: [], collateral: [], bound_tools: [], default_runtime: false, refuses: false };
    if (typeof value !== 'object') return unknown;
    const v = value as Record<string, unknown>;
    if (typeof v.state !== 'string' || !SIMULATION_STATES.has(v.state) || !isStrings(v.remove) || !isStrings(v.bound_tools)
        || typeof v.default_runtime !== 'boolean' || typeof v.refuses !== 'boolean' || !Array.isArray(v.collateral)) return unknown;
    const collateral: RemovalSimulation['collateral'] = [];
    for (const item of v.collateral) {
        const c = item as Record<string, unknown> | null;
        if (!c || typeof c !== 'object' || typeof c.package !== 'string' || typeof c.reason !== 'string' || !COLLATERAL_REASONS.has(c.reason)) return unknown;
        collateral.push({ package: c.package, reason: c.reason as RemovalSimulation['collateral'][number]['reason'], component: typeof c.component === 'string' ? c.component : '' });
    }
    if (!v.bound_tools.every((tool) => PHP_TOOL_SERVICE[tool] !== undefined)) return unknown;
    return {
        state: v.state as RemovalSimulation['state'], remove: v.remove, collateral, bound_tools: v.bound_tools,
        default_runtime: v.default_runtime, refuses: v.refuses,
        ...(v.keeps_data === true ? { keeps_data: true, data_paths: isStrings(v.data_paths) ? v.data_paths : [] } : {}),
    };
}
const REMOVAL_HOSTED_KINDS: ReadonlySet<string> = new Set(['sites', 'php_sites', 'node_sites', 'databases', 'mail_addresses', 'mail_domains', 'mailboxes', 'certificates', 'firewall', 'unrecorded', 'vpn_peers', 'ftp_accounts']);
// How long the dialog waits for the read before it says it could not read.
// The read includes the package manager's own simulation, which the Panel
// bounds at 30 seconds (alpha.104).
// Okumanın sınırı; sonra okunamadığını söyler. Okuma paket yöneticisinin
// benzetimini de içerir.
export const REMOVAL_IMPACT_READ_BOUND_MS = 40_000;
// How many hosted names the dialog shows; the rest is "+N more".
export const REMOVAL_IMPACT_NAMES_SHOWN = 5;

const isStrings = (value: unknown): value is string[] => Array.isArray(value) && value.every((item) => typeof item === 'string');

// The answer's shape is the contract; anything else is "could not be read".
// Yanıtın biçimi sözleşmedir; başka her şey "okunamadı"dır.
export function decodeRemovalImpact(value: unknown): RemovalImpact | null {
    if (!value || typeof value !== 'object') return null;
    const v = value as Record<string, unknown>;
    if (v.removal !== 'packages' && v.removal !== 'webmail_tree' && v.removal !== 'unsupported') return null;
    if (typeof v.has_units !== 'boolean' || typeof v.setup_required !== 'boolean') return null;
    if (typeof v.package_family !== 'string' || !isStrings(v.packages) || !isStrings(v.firewall_ports) || !isStrings(v.required_by)) return null;
    if (typeof v.all_versions !== 'boolean' || typeof v.mail_filter_rewired !== 'boolean' || typeof v.database_tools_reconfigured !== 'boolean') return null;
    if (!Array.isArray(v.bundles) || !v.bundles.every((b) => b && typeof b === 'object' && typeof (b as { id?: unknown }).id === 'string' && typeof (b as { name_code?: unknown }).name_code === 'string')) return null;
    let hosted: RemovalImpact['hosted'] = null;
    if (v.hosted !== null) {
        const h = v.hosted as Record<string, unknown> | undefined;
        if (!h || typeof h !== 'object' || typeof h.kind !== 'string' || !REMOVAL_HOSTED_KINDS.has(h.kind)) return null;
        if (typeof h.count !== 'number' || !Number.isInteger(h.count) || h.count < 0 || !isStrings(h.names) || typeof h.blocks_removal !== 'boolean') return null;
        hosted = { kind: h.kind as RemovalHostedKind, count: h.count, names: h.names, blocks_removal: h.blocks_removal };
        if (h.firewall !== undefined) {
            const f = h.firewall as Record<string, unknown> | null;
            if (!f || typeof f !== 'object' || typeof f.known !== 'boolean' || typeof f.enabled !== 'boolean'
                || typeof f.open_ports !== 'number' || !Number.isInteger(f.open_ports) || f.open_ports < 0) return null;
            hosted.firewall = { known: f.known, enabled: f.enabled, open_ports: f.open_ports };
        }
    }
    return {
        removal: v.removal,
        has_units: v.has_units,
        setup_required: v.setup_required,
        package_family: v.package_family,
        packages: v.packages,
        all_versions: v.all_versions,
        firewall_ports: v.firewall_ports,
        mail_filter_rewired: v.mail_filter_rewired,
        database_tools_reconfigured: v.database_tools_reconfigured,
        required_by: v.required_by,
        bundles: (v.bundles as { id: string; name_code: string }[]).map((b) => ({ id: b.id, name_code: b.name_code })),
        hosted,
        simulation: decodeRemovalSimulation(v.simulation),
        version: typeof v.version === 'string' ? v.version : '',
    };
}

type RemovalRead = { state: 'checking' } | { state: 'known'; impact: RemovalImpact } | { state: 'unknown' } | { state: 'refused'; error: ApiError };
// A component the Panel never removes from this dialog answers with its own
// refusal: that is a known answer, said in its own words, not "could not be read".
// Panel'in bu pencereden hiç kaldırmadığı bileşen kendi reddiyle yanıtlar.
const REMOVAL_REFUSAL_CODES: ReadonlySet<string> = new Set(['NATIVE_CRON_REMOVAL_REFUSED', 'DNS_ENGINE_WORKFLOW_REQUIRED']);

// One read when the dialog opens, bounded; Retry reads again. A read never
// starts a change.
// Pencere açılınca tek okuma, sınırlı; Yeniden dene yine yalnız okur.
function useRemovalImpact(serviceId: string, enabled: boolean, packageName = ''): { read: RemovalRead; retry: () => void } {
    const [read, setRead] = useState<RemovalRead>({ state: 'checking' });
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        if (!enabled) return;
        let settled = false;
        const controller = typeof AbortController === 'undefined' ? null : new AbortController();
        setRead({ state: 'checking' });
        const finish = (next: RemovalRead) => {
            if (settled) return;
            settled = true;
            setRead(next);
        };
        const bound = setTimeout(() => {
            controller?.abort();
            finish({ state: 'unknown' });
        }, REMOVAL_IMPACT_READ_BOUND_MS);
        (async () => {
            try {
                const query = `service_id=${encodeURIComponent(serviceId)}${packageName ? `&package=${encodeURIComponent(packageName)}` : ''}`;
                const response = await fetch(`/api/v1/service/removal-impact?${query}`, {
                    cache: 'no-store',
                    ...(controller ? { signal: controller.signal } : {}),
                });
                if (!response.ok) {
                    const error = await readApiError(response);
                    finish(error.code && REMOVAL_REFUSAL_CODES.has(error.code) ? { state: 'refused', error } : { state: 'unknown' });
                    return;
                }
                const impact = decodeRemovalImpact(await response.json());
                finish(impact ? { state: 'known', impact } : { state: 'unknown' });
            } catch {
                finish({ state: 'unknown' });
            } finally {
                clearTimeout(bound);
            }
        })();
        return () => {
            settled = true;
            clearTimeout(bound);
            controller?.abort();
        };
    }, [serviceId, enabled, attempt, packageName]);
    return { read, retry: () => setAttempt((n) => n + 1) };
}

// The removed and kept sentences, by what the uninstall path does for this
// component (service_install_handler.go handleServiceUninstall →
// Agent.UninstallService / Agent.RemoveRoundcube; package_removal_plan.go
// since alpha.104: apt-get purge with the exact simulated list, pacman -Rns
// --noconfirm). Each key is a sentence this dialog may say.
// Silinen ve kalan cümleler, kaldırma yolunun bu bileşen için yaptığına göre.
export function removalSentences(impact: RemovalImpact): { removed: TranslationKey[]; kept: TranslationKey[]; warnings: TranslationKey[]; command: string | null } {
    if (impact.removal === 'webmail_tree') {
        return { removed: ['services.removal.removed.webmailTree'], kept: ['services.removal.kept.webmailTree'], warnings: [], command: null };
    }
    // No catalogue packages on this package manager: the Agent refuses, so
    // nothing is said to be removed (Agent.UninstallService).
    if (impact.removal === 'unsupported') {
        return { removed: ['services.removal.removed.unsupported'], kept: [], warnings: [], command: null };
    }
    // Units are stopped first only when the catalogue names some; a tool
    // without units has nothing to stop. Since alpha.104 apt purges exactly
    // the list its own simulation showed: the packages, the packages of the
    // removed version and what only this removal leaves unused; pacman -Rns
    // takes the dependencies nothing else needs.
    const removed: TranslationKey[] = impact.has_units ? ['services.removal.removed.stopUnits'] : [];
    let command: string | null = null;
    // alpha.107 (D-038 §8): a database engine is removed, not purged: its
    // clusters, databases and configuration stay where the dialogue says.
    if (impact.simulation?.keeps_data) {
        removed.push(impact.package_family === 'pacman' ? 'services.removal.removed.pacman' : 'services.removal.removed.aptKeepData');
        if (impact.all_versions) removed.push('services.removal.removed.allVersions');
        return {
            removed,
            kept: ['services.removal.kept.engineData'],
            warnings: [],
            command: impact.package_family === 'pacman' ? 'pacman -Rns' : 'apt-get remove',
        };
    }
    if (impact.package_family === 'apt') {
        removed.push('services.removal.removed.apt');
        command = 'apt-get purge';
    } else if (impact.package_family === 'pacman') {
        removed.push('services.removal.removed.pacman');
        command = 'pacman -Rns';
    } else {
        removed.push('services.removal.removed.packages');
    }
    if (impact.all_versions) removed.push('services.removal.removed.allVersions');
    if (impact.mail_filter_rewired) removed.push('services.removal.removed.mailFilter');
    if (impact.database_tools_reconfigured) removed.push('services.removal.removed.databaseTools');
    const warnings: TranslationKey[] = impact.hosted?.kind === 'databases' ? ['services.removal.warn.databaseFiles'] : [];
    return { removed, kept: ['services.removal.kept.packages'], warnings, command };
}

// One line of a removal answer (alpha.106): a cause, a next action, a note
// or a detail. `line`, `sentence` and `sim` keep the names the evidence and
// the tests read; an action is a link to the page where it is taken, or a
// button that opens the next removal.
// Kaldırma yanıtının tek satırı: neden, sonraki eylem, not ya da ayrıntı.
type RemovalItem = {
    key: string;
    text: string;
    line?: string;
    sentence?: string;
    sim?: string;
    to?: string;
    linkText?: string;
    onClick?: () => void;
};
type RemovalFindings = { causes: RemovalItem[]; steps: RemovalItem[]; details: RemovalItem[]; notes: RemovalItem[]; named: Set<string> };
type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

// What the package manager's answer means for this removal (alpha.104; since
// alpha.106 the answer first, D-024): each cause once, in plain words; the
// next action beside it, with the way to take it; the package manager's own
// explanation only in Details. A read that could not run is a note and the
// removal stays allowed (the Agent checks again when it is pressed).
// Paket yöneticisinin yanıtı: her neden bir kez, sade sözle; sonraki eylem
// yanında; paket yöneticisinin açıklaması yalnız Ayrıntılar'da.
export function simulationFindings(t: Translate, { name, whole, serviceId, simulation, nameOf, version, onRemoveComponent, onRemoveWhole }: {
    name: string;
    whole: string;
    serviceId: string;
    simulation: RemovalSimulation;
    nameOf: (id: string) => string;
    version: string;
    onRemoveComponent?: (id: string) => void;
    onRemoveWhole?: () => void;
}): RemovalFindings {
    const causes: RemovalItem[] = [];
    const steps: RemovalItem[] = [];
    const details: RemovalItem[] = [];
    const notes: RemovalItem[] = [];
    const named = new Set<string>();
    const removeFirst = (component: string) => {
        if (named.has(component)) return;
        named.add(component);
        steps.push({
            key: `remove-${component}`,
            text: t('services.removal.next.remove', { component: nameOf(component) }),
            ...(onRemoveComponent ? { onClick: () => onRemoveComponent(component) } : {}),
        });
    };
    const tools = simulation.bound_tools.map((tool) => PHP_TOOL_SERVICE[tool] ?? tool);
    if (tools.length > 0) {
        const names = tools.map(nameOf).join(', ');
        causes.push({ key: 'tools', sim: 'tools', text: version
            ? t('services.removal.why.tools', { names, version })
            : t('services.removal.why.toolsAll', { names, name }) });
        // A version's tools move to another version (said by the move
        // itself); a whole component's tools are removed first.
        // Sürümün araçları taşınır; bileşenin tamamında araçlar önce kaldırılır.
        for (const tool of tools) {
            if (version) named.add(tool);
            else removeFirst(tool);
        }
    }
    if (simulation.state === 'checked' && simulation.collateral.length > 0) {
        const byComponent = new Map<string, string[]>();
        const depends: string[] = [];
        const installs: string[] = [];
        let own: string[] = [];
        for (const item of simulation.collateral) {
            if (item.reason === 'component' && item.component === serviceId) {
                own = [...own, item.package];
            } else if (item.reason === 'component' && item.component) {
                byComponent.set(item.component, [...(byComponent.get(item.component) ?? []), item.package]);
            } else if (item.reason === 'would_install') {
                installs.push(item.package);
            } else {
                depends.push(item.package);
            }
        }
        for (const [component, packages] of byComponent) {
            causes.push({ key: `component-${component}`, sim: `component-${component}`, text: t('services.removal.why.component', { component: nameOf(component), packages: packages.join(', ') }) });
            removeFirst(component);
        }
        if (own.length > 0) {
            causes.push({ key: 'default', sim: 'default', text: t('services.removal.why.default', { packages: own.join(', '), name: whole }) });
            if (version) steps.push({ key: 'remove-all', text: t('services.removal.next.removeAll', { name: whole }), ...(onRemoveWhole ? { onClick: onRemoveWhole } : {}) });
        }
        if (depends.length > 0) {
            causes.push({ key: 'depends', sim: 'depends', text: t('services.removal.why.depends', { packages: depends.join(', ') }) });
            steps.push({ key: 'depends', text: t('services.removal.next.depends') });
        }
        details.push({ key: 'collateral', sim: 'collateral', text: t('services.removal.sim.refused', { name }) });
        if (installs.length > 0) details.push({ key: 'installs', sim: 'installs', text: t('services.removal.sim.installs', { packages: installs.join(', ') }) });
    }
    if (simulation.refuses && causes.length === 0) causes.push({ key: 'other', text: t('services.removal.why.other') });
    if (simulation.state === 'busy') notes.push({ key: 'busy', sim: 'busy', text: t('services.removal.sim.busy') });
    if (simulation.state === 'unknown') notes.push({ key: 'unknown', sim: 'unknown', text: t('services.removal.sim.unknown') });
    return { causes, steps, details, notes, named };
}

const NO_FINDINGS: RemovalFindings = { causes: [], steps: [], details: [], notes: [], named: new Set() };

// Whether the Panel would refuse this removal as things are: no package list
// on this package manager (Agent.UninstallService), what it hosts
// (SERVICE_HAS_DEPENDENTS) or the package manager's answer (alpha.104). The
// dialog then says so first and offers Close, not Remove (alpha.106).
// Panel bu kaldırmayı şu durumda reddeder mi.
// Where a database engine's data stays (alpha.107): the paths the Agent
// found, or the engine's usual place when it found none yet.
export function removalDataPathsText(impact: RemovalImpact): string {
    const paths = impact.simulation?.data_paths ?? [];
    if (paths.length > 0) return paths.join(', ');
    return impact.package_family === 'pacman' ? '/var/lib/postgres, /var/lib/mysql' : '/var/lib/postgresql, /var/lib/mysql';
}

export function removalRefused(impact: RemovalImpact): boolean {
    return impact.removal === 'unsupported'
        || Boolean(impact.hosted?.blocks_removal && impact.hosted.count > 0)
        || impact.simulation?.refuses === true;
}

function RemovalItemText({ item }: { item: RemovalItem }) {
    const text = (
        <span data-removal-line={item.line} data-removal-sentence={item.sentence} data-removal-sim={item.sim}>
            <KeepWords text={item.text} />
        </span>
    );
    if (item.onClick) {
        return (
            <button type="button" onClick={item.onClick} data-removal-action={item.key}
                className="inline-flex items-start text-left align-top font-medium text-primary underline underline-offset-2 hover:no-underline max-sm:min-h-11">
                {text}
            </button>
        );
    }
    return (
        <>
            {text}
            {item.to && <>{' '}<Link to={item.to} className="text-primary underline">{item.linkText}</Link></>}
        </>
    );
}

// Every removal dialog in one order (alpha.106, D-024): the answer first —
// can it be removed now, and why not, one line per cause; then what the
// owner can do, with the way to do it; what the removal affects; and the
// technical part (package list, what apt or pacman does, the package
// manager's own explanation) folded under Details. A refused removal is
// never described as what it "takes".
// Her kaldırma penceresi aynı sırada: önce yanıt ve nedenleri; sonra
// yapılabilecekler; etkilenenler; teknik kısım Ayrıntılar altında.
function RemovalAnswer({ verdict, subject, causes, steps, notes, children, details }: {
    verdict: 'blocked' | 'move' | 'allowed';
    subject: string;
    causes: RemovalItem[];
    steps: RemovalItem[];
    notes: RemovalItem[];
    children?: ReactNode;
    details?: ReactNode;
}) {
    const { t } = useI18n();
    const all = verdict === 'allowed' ? steps : [...steps, { key: 'keep', text: t('services.removal.next.keep', { name: subject }) }];
    return (
        <div className="space-y-3 text-sm" data-removal-verdict={verdict}>
            <div className={verdict === 'allowed' ? undefined : 'rounded-lg border border-warning-mark/50 bg-warning-mark/20 px-3 py-2'} data-removal-answer="">
                <p className="text-base font-semibold text-fg">
                    <KeepWords text={t(`services.removal.verdict.${verdict}` as TranslationKey, { name: subject })} />
                </p>
                {causes.length > 0 && (
                    <ul className="mt-1 list-disc space-y-1 pl-5 text-fg" data-removal-reasons="">
                        {causes.map((item) => <li key={item.key} data-removal-cause={item.key}><RemovalItemText item={item} /></li>)}
                    </ul>
                )}
            </div>
            {notes.map((item) => <p key={item.key} className="text-fg-muted" data-removal-note={item.key}><RemovalItemText item={item} /></p>)}
            {all.length > 0 && (
                <div data-removal-next="">
                    <p className="font-semibold text-fg">{t('services.removal.nextTitle')}</p>
                    <ul className="mt-1 list-disc space-y-1 pl-5 text-fg">
                        {all.map((item) => <li key={item.key} data-removal-step={item.key}><RemovalItemText item={item} /></li>)}
                    </ul>
                </div>
            )}
            {children}
            {details && (
                <details className="rounded-lg border border-border px-3 py-2" data-removal-details="">
                    <summary className="cursor-pointer select-none font-medium text-fg max-sm:py-2">{t('services.removal.detailsTitle')}</summary>
                    <div className="mt-2 space-y-3">{details}</div>
                </details>
            )}
        </div>
    );
}

function PackageChips({ packages }: { packages: string[] }) {
    return (
        <div className="flex flex-wrap gap-1.5">
            {packages.map((pkg) => <span key={pkg} className="rounded bg-surface-2 px-2 py-0.5 font-mono text-xs text-fg">{pkg}</span>)}
        </div>
    );
}

// The package manager's checked list, in Details: what an allowed removal
// takes, or what a refused one would have taken.
// Paket yöneticisinin denetlediği liste, Ayrıntılar'da.
function SimulatedList({ simulation, refused }: { simulation: RemovalSimulation | null; refused: boolean }) {
    const { t } = useI18n();
    if (simulation?.state !== 'checked' || simulation.remove.length === 0) return null;
    return (
        <div data-removal-exact="">
            <p className="mb-1 text-fg-muted">{t(refused ? 'services.removal.sim.wouldTake' : 'services.removal.sim.exact', { count: simulation.remove.length })}</p>
            <PackageChips packages={simulation.remove} />
        </div>
    );
}

function RemovalImpactSection({
    name,
    subject,
    serviceId,
    read,
    onRetry,
    nameOf,
    bundleNameOf,
    packages,
    onRemoveComponent,
}: {
    name: string;
    /** What this removal removes, as the title names it. / Başlığın adlandırdığı. */
    subject: string;
    serviceId: string;
    read: RemovalRead;
    onRetry: () => void;
    nameOf: (id: string) => string;
    bundleNameOf: (id: string, nameCode: string) => string;
    packages: string[];
    onRemoveComponent?: (id: string) => void;
}) {
    const { t } = useI18n();
    const chips = (
        <div data-removal-packages="">
            <PackageChips packages={packages} />
        </div>
    );
    if (read.state === 'checking') {
        return (
            <div className="space-y-3" data-removal-impact="checking">
                <Checking label={t('services.removal.checking', { name })} />
                {chips}
            </div>
        );
    }
    if (read.state === 'refused') {
        return (
            <div data-removal-impact="refused">
                <RemovalAnswer verdict="blocked" subject={subject} causes={[]} steps={[]} notes={[]}>
                    <ErrorBanner error={read.error} />
                </RemovalAnswer>
            </div>
        );
    }
    if (read.state === 'unknown') {
        return (
            <div className="space-y-3" data-removal-impact="unknown">
                <CouldNotCheck text={t('services.removal.unknown', { name })} onRetry={onRetry} />
                {/* Roundcube is not a package: its removal deletes the webmail tree. */}
                <p className="text-sm text-fg-muted">{t(serviceId === 'roundcube' ? 'services.removal.removed.webmailTree' : 'services.uninstallWarn', { name })}</p>
                {chips}
            </div>
        );
    }
    const { impact } = read;
    const sentences = removalSentences(impact);
    const simulation = impact.simulation;
    const refused = removalRefused(impact);
    const findings = simulation
        ? simulationFindings(t, { name, whole: name, serviceId, simulation, nameOf, version: impact.version, onRemoveComponent })
        : NO_FINDINGS;
    // The firewall and the unrecorded kinds carry no list; a counted kind
    // with nothing in it says nothing.
    const hosted = impact.hosted && impact.hosted.count > 0 && !['firewall', 'unrecorded'].includes(impact.hosted.kind) ? impact.hosted : null;
    const shown = hosted ? hosted.names.slice(0, REMOVAL_IMPACT_NAMES_SHOWN) : [];
    const more = hosted ? hosted.count - shown.length : 0;
    const causes: RemovalItem[] = [];
    const steps: RemovalItem[] = [];
    const affected: RemovalItem[] = [];
    if (impact.removal === 'unsupported') {
        causes.push({ key: 'unsupported', sentence: 'services.removal.removed.unsupported', text: t('services.removal.removed.unsupported', { name }) });
    }
    if (hosted) {
        const line: RemovalItem = {
            key: 'hosted',
            line: 'hosted',
            text: t(`services.removal.hosted.${hosted.kind}` as TranslationKey, {
                name,
                count: hosted.count,
                names: shown.join(', ') + (more > 0 ? (shown.length > 0 ? ' ' : '') + t('services.removal.more', { n: more }) : ''),
            }),
        };
        if (hosted.blocks_removal) {
            causes.push(line);
            const to = ['sites', 'php_sites', 'node_sites'].includes(hosted.kind) ? '/domains' : hosted.kind === 'databases' ? '/databases' : '';
            steps.push({
                key: 'blocks',
                line: 'blocks',
                text: t('services.removal.blocks', { name }),
                ...(to ? { to, linkText: t(to === '/domains' ? 'services.removal.next.sitesLink' : 'services.removal.next.databasesLink') } : {}),
            });
        } else {
            affected.push(line);
        }
    }
    causes.push(...findings.causes);
    steps.push(...findings.steps);
    if (impact.hosted?.kind === 'firewall') {
        const firewall = impact.hosted.firewall;
        affected.push({
            key: 'firewall',
            line: 'firewall',
            text: !firewall?.known
                ? t('services.removal.hosted.firewall.unknown', { name })
                : firewall.enabled
                    ? t('services.removal.hosted.firewall.on', { name, n: firewall.open_ports })
                    : t('services.removal.hosted.firewall.off', { name }),
        });
    }
    if (impact.hosted?.kind === 'unrecorded') affected.push({ key: 'unrecorded', line: 'unrecorded', text: t('services.removal.hosted.unrecorded', { name }) });
    if (impact.setup_required) affected.push({ key: 'setup', line: 'setup', text: t('services.removal.setupRequired', { name }) });
    // A component already named as a cause is not said again (alpha.106).
    // Neden olarak adı geçen bileşen yeniden söylenmez.
    const requiredBy = impact.required_by.filter((id) => !findings.named.has(id));
    if (requiredBy.length > 0) {
        affected.push({ key: 'required', line: 'required', text: t('services.removal.requiredBy', { name, names: requiredBy.map(nameOf).join(', ') }) });
    }
    for (const bundle of impact.bundles) {
        affected.push({ key: `bundle-${bundle.id}`, line: `bundle-${bundle.id}`, text: t('services.removal.bundle', { bundle: bundleNameOf(bundle.id, bundle.name_code) }) });
    }
    const nothing = affected.length === 0 && causes.length === 0;
    if (impact.firewall_ports.length > 0) {
        affected.push({ key: 'ports', line: 'ports', text: t('services.removal.ports', { ports: impact.firewall_ports.join(', ') }) });
    }
    const affectedList = (title: TranslationKey) => (
        <div data-removal-affected="">
            <p className="font-semibold text-fg">{t(title)}</p>
            {nothing && <p className="mt-1 text-fg-muted" data-removal-nothing="">{t('services.removal.nothing', { name })}</p>}
            {affected.length > 0 && (
                <ul className="mt-1 list-disc space-y-1 pl-5 text-fg">
                    {affected.map((item) => <li key={item.key}><RemovalItemText item={item} /></li>)}
                </ul>
            )}
        </div>
    );
    const warnings = sentences.warnings.map((key) => (
        <p key={key} className="rounded-lg border border-warning-mark/50 bg-warning-mark/20 px-3 py-2 text-fg" data-removal-warning={key}>{t(key, { name })}</p>
    ));
    // Without a package list nothing is removed: the cause says so and
    // Details has nothing to add.
    // Paket listesi yoksa hiçbir şey kaldırılmaz; neden bunu söyler.
    const removedPart = impact.removal === 'unsupported' ? null : (
        <div data-removal-removed="">
            <p className="font-semibold text-fg">{t(refused ? 'services.removal.wouldTitle' : 'services.removal.removedTitle')}</p>
            {sentences.removed.map((key) => <p key={key} className="mt-1 text-fg-muted" data-removal-sentence={key}>{t(key, { name })}</p>)}
            {findings.details.map((item) => <p key={item.key} className="mt-1 text-fg-muted"><RemovalItemText item={item} /></p>)}
            <div className="mt-2">
                {simulation?.state === 'checked' && simulation.remove.length > 0
                    ? <SimulatedList simulation={simulation} refused={refused} />
                    : impact.packages.length > 0 && chips}
            </div>
            {sentences.command && (
                <p className="mt-1 text-xs text-fg-subtle" data-removal-command="">
                    {t('services.removal.command')} <span className="font-mono">{sentences.command}</span>
                </p>
            )}
            {refused && warnings.length > 0 && <div className="mt-2">{warnings}</div>}
        </div>
    );
    const keptPart = sentences.kept.length > 0 && (
        <div data-removal-kept="">
            <p className="font-semibold text-fg">{t('services.removal.keptTitle')}</p>
            {sentences.kept.map((key) => <p key={key} className="mt-1 text-fg-muted" data-removal-sentence={key}><KeepWords text={t(key, { name, paths: removalDataPathsText(impact) })} /></p>)}
        </div>
    );
    return (
        // The answer first (D-024): can it go now, why not, what to do; what
        // the server loses; the technical part under Details. Red stays on the
        // Remove button only.
        // Önce yanıt; sonra kaybedilenler; teknik kısım Ayrıntılar altında.
        <div data-removal-impact="known">
            <RemovalAnswer
                verdict={refused ? 'blocked' : 'allowed'}
                subject={subject}
                causes={causes}
                steps={steps}
                notes={findings.notes}
                details={(refused && affected.length > 0) || removedPart || keptPart ? (
                    <>
                        {refused && affected.length > 0 && affectedList('services.removal.alsoTitle')}
                        {removedPart}
                        {keptPart}
                    </>
                ) : undefined}
            >
                {!refused && affectedList('services.removal.affectedTitle')}
                {!refused && warnings}
            </RemovalAnswer>
        </div>
    );
}

// A refused removal's buttons (alpha.106): Close is the action; the danger
// button stays, visibly unavailable, and says why in its own label.
// Reddedilen kaldırmanın düğmeleri: Kapat asıl eylemdir; tehlike düğmesi
// görünür biçimde kullanılamaz ve etiketinde nedenini söyler.
function RefusedActions({ onClose, busy }: { onClose: () => void; busy: boolean }) {
    const { t } = useI18n();
    return (
        <>
            <Button variant="danger" disabled title={t('services.removal.refusedButton')} data-removal-cant="">{t('services.removal.cantButton')}</Button>
            <Button variant="primary" onClick={onClose} disabled={busy} data-removal-close="">{t('common.close')}</Button>
        </>
    );
}

// The versions a runtime row's remove button can remove one at a time: the
// managed copies with their own unit (php8.4-fpm). Node's trees are removed
// from the versions list only.
// Satırın kaldır düğmesinin tek tek kaldırabileceği sürümler.
function removableVersions(service: ManagedService): string[] {
    if (service.kind !== 'runtime') return [];
    return (service.instances ?? []).filter((inst) => inst.managed && inst.unit).map((inst) => inst.version);
}

// "Which version do you want to remove?" (alpha.106): one choice per
// installed version with who uses it (the alpha.104 usage read), and the
// whole component, every version, set apart below. Choosing only continues
// to the removal's own dialog; nothing changes here.
// "Hangi sürüm kaldırılsın?": kurulu her sürüm için bir seçim ve kimin
// kullandığı; bileşenin tamamı ayrı, altta. Seçim yalnız sonraki pencereye geçer.
function RemoveWhichVersionDialog({ service, onCancel, onPickVersion, onPickAll }: {
    service: ManagedService;
    onCancel: () => void;
    onPickVersion: (version: string) => void;
    onPickAll: () => void;
}) {
    const { t } = useI18n();
    const name = componentName(service, t);
    const versions = removableVersions(service);
    const usage = service.id === 'php-fpm';
    const [tools, setTools] = useState<PHPToolsInfo | null>(null);
    useEffect(() => {
        if (!usage) return;
        const controller = typeof AbortController === 'undefined' ? null : new AbortController();
        void readPHPTools(controller?.signal).then((info) => { if (!controller?.signal.aborted) setTools(info); });
        return () => controller?.abort();
    }, [usage]);
    const toolName = (tool: string) => componentName({ id: PHP_TOOL_SERVICE_ID[tool as keyof typeof PHP_TOOL_SERVICE_ID] ?? tool, name: tool }, t);
    const choice = 'flex w-full flex-col items-start gap-0.5 rounded-lg border px-3 py-2 text-left transition-colors max-sm:min-h-11';
    return (
        <Dialog
            id="service-remove-which"
            icon={Trash2}
            title={t('services.removeWhich.title', { name })}
            description={t('services.removeWhich.description')}
            onDismiss={onCancel}
            actions={<Button variant="secondary" onClick={onCancel}>{t('common.cancel')}</Button>}
        >
            <ul className="space-y-2" data-remove-which="">
                {versions.map((version) => (
                    <li key={version}>
                        <button type="button" onClick={() => onPickVersion(version)} data-remove-which-version={version}
                            className={`${choice} border-border-strong bg-surface hover:bg-surface-2`}>
                            <span className="font-medium text-fg">{name} {version}</span>
                            {usage && (tools === null
                                ? <span className="text-xs text-fg-muted">{t('services.removeWhich.reading')}</span>
                                : <VersionUsage info={tools} version={version} toolName={toolName} />)}
                        </button>
                    </li>
                ))}
            </ul>
            <div className="mt-3 border-t border-border pt-3">
                <button type="button" onClick={onPickAll} data-remove-which-all=""
                    className={`${choice} border-danger/40 bg-surface hover:bg-danger/10`}>
                    <span className="font-medium text-danger">{t('services.removeWhich.all', { versions: versions.join(', ') })}</span>
                    <span className="text-xs text-fg-muted">{t('services.removeWhich.allHint', { name })}</span>
                </button>
            </div>
        </Dialog>
    );
}

// Themed uninstall confirmation — a destructive action, so it states plainly
// what will be purged and that dependent sites/mail may break. Removing a
// service is how the operator shrinks the attack surface back down.
// Temalı kaldırma onayı — yıkıcı bir eylem; ne purge edileceğini ve bağımlı
// site/postanın çalışmayabileceğini açıkça söyler. Bir servisi kaldırmak,
// operatörün saldırı yüzeyini geri küçültme yoludur.
function UninstallServiceDialog({
    service,
    busy,
    error,
    retryCleanup,
    cleanupAttempt,
    onCancel,
    onConfirm,
    nameOf,
    bundleNameOf,
    onRemoveComponent,
}: {
    service: ManagedService;
    busy: boolean;
    error: ApiError | null;
    retryCleanup: boolean;
    cleanupAttempt: 'initial' | 'retry' | null;
    onCancel: () => void;
    onConfirm: () => void;
    nameOf: (id: string) => string;
    bundleNameOf: (id: string, nameCode: string) => string;
    onRemoveComponent?: (id: string) => void;
}) {
    const { t } = useI18n();
    // The leftovers cleanup has its own fixed list; only a removal reads
    // what it affects.
    const impact = useRemovalImpact(service.id, !retryCleanup);
    // alpha.104: a removal the Panel would refuse is not offered; since
    // alpha.106 the dialog says so first, the button reads "Can't remove"
    // and Close is the action.
    // Panel'in reddedeceği kaldırma sunulmaz; pencere bunu önce söyler.
    const refused = !retryCleanup && (impact.read.state === 'refused' || (impact.read.state === 'known' && removalRefused(impact.read.impact)));
    // The title names what goes (alpha.106, owner's question "which version
    // would it remove?"): every installed version, or the one there is.
    // Başlık neyin gideceğini söyler: kurulu bütün sürümler ya da tek sürüm.
    const name = componentName(service, t);
    const versions = removableVersions(service);
    const subject = versions.length > 1 ? t('services.removal.subjectAll', { name, versions: versions.join(', ') })
        : versions.length === 1 ? `${name} ${versions[0]}` : name;
    // Destructive: no silent exit — see the note on the version-removal dialog.
    // dismissible={false} takes the backdrop and Escape away together.
    // Yıkıcı: sessiz çıkış yok — sürüm kaldırma diyalogundaki nota bakın.
    return (
        <Dialog
            id="service-uninstall"
            tone="danger"
            icon={Trash2}
            dismissible={false}
            busy={busy}
            title={retryCleanup
                ? t('services.cleanupWebmailTitle')
                : versions.length > 1 ? t('services.removeSubjectTitle', { name: subject })
                : versions.length === 1 ? t('services.removeVersionTitle', { name, version: versions[0] })
                : t('services.uninstallTitle', { name })}
            description={componentDescription(service, t)}
            actions={refused ? <RefusedActions onClose={onCancel} busy={busy} /> : (
                <>
                    <Button variant="secondary" onClick={onCancel} disabled={busy}>{t('common.cancel')}</Button>
                    <Button variant="danger" onClick={onConfirm} disabled={busy} icon={Trash2}>
                        {busy
                            ? t(retryCleanup
                                ? (cleanupAttempt === 'retry' ? 'services.retryingWebmailCleanup' : 'services.cleaningWebmail')
                                : 'services.uninstalling')
                            : t(retryCleanup
                                ? (error ? 'services.retryWebmailCleanup' : 'services.cleanupWebmail')
                                : 'services.uninstall')}
                    </Button>
                </>
            )}
        >
                {!retryCleanup && (
                    <div className="mb-4">
                        <RemovalImpactSection
                            name={name}
                            subject={subject}
                            serviceId={service.id}
                            read={impact.read}
                            onRetry={impact.retry}
                            nameOf={nameOf}
                            bundleNameOf={bundleNameOf}
                            packages={service.id === 'roundcube' ? [] : service.packages && service.packages.length > 0 ? service.packages : [service.id]}
                            onRemoveComponent={onRemoveComponent}
                        />
                    </div>
                )}
                {retryCleanup && (
                <div className="mb-4 rounded-lg border border-danger/30 bg-danger/5 p-3 text-sm text-fg-muted">
                    <p className="mb-2">
                        {t('services.cleanupWebmailWarn')}
                    </p>
                    {/* The concrete list the cleanup removes, as the Agent
                        names them (cmd/agent/webmail_rpc.go,
                        webmail_files_linux.go; a test keeps them equal), and
                        what it never touches.
                        Temizliğin sildiği somut liste ve asla dokunmadığı şeyler. */}
                    {retryCleanup && (
                        <>
                            <ul className="mb-2 space-y-1.5" data-webmail-cleanup-list="">
                                {WEBMAIL_CLEANUP_PATHS.map(({ paths, key }) => (
                                    <li key={key} className="flex flex-col gap-0.5">
                                        {paths.map((path) => <span key={path} className="break-all font-mono text-xs text-fg">{path}</span>)}
                                        <span className="text-xs">{t(key)}</span>
                                    </li>
                                ))}
                            </ul>
                            <p data-webmail-cleanup-untouched=""><KeepWords text={t('services.cleanupWebmailUntouched')} /></p>
                        </>
                    )}
                </div>
                )}
                {/* A refusal shows its evidence HERE, where the decision is
                    made — who blocks, line by line (B3d).
                    Ret, kanıtını kararın verildiği yerde gösterir — kimin
                    engellediği, satır satır (B3d). */}
                <ErrorBanner error={error} />
        </Dialog>
    );
}
