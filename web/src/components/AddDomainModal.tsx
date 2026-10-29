import { useState } from 'react';
import { useNavigate } from '../router';
import { Globe, Lock, Server, Network } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { Button, Checking, CouldNotCheck, Dialog, ErrorBanner } from './ui';
import { readApiError, apiErrorText, type ApiError } from '../lib/apiError';
import { isSiteWebServerRefused, siteWebServerRefusedIn } from '../lib/siteWebServerRefused';
import { gateOn } from '../lib/remote';
import {
    ISSUE_FAILURE_KEYS,
    isIssueFailure,
    requestSiteCertificate,
    type SiteCertificateOutcome,
} from '../lib/siteCertificateRequest';
import {
    dnsBlocker,
    hostingDNSReady,
    localDNSReady,
    useHostingCapabilities,
    type DomainPurpose,
} from '../lib/hostingCapabilities';

/**
 * The site's names when it took over the infrastructure zone (alpha.115):
 * the addresses added, and the names that already point elsewhere and were
 * left exactly as they are.
 */
export interface SiteRecordsOutcome {
    added: { name: string; type: string; content: string }[];
    kept: { name: string; type: string; content: string; expected?: string }[];
    ipv4: string;
}

function readSiteRecords(value: unknown): SiteRecordsOutcome | undefined {
    if (!value || typeof value !== 'object') return undefined;
    const site = (value as Record<string, unknown>).site_records;
    if (!site || typeof site !== 'object') return undefined;
    const record = site as Record<string, unknown>;
    const rows = (list: unknown) => Array.isArray(list) && list.length <= 8 && list.every((row) => !!row && typeof row === 'object'
        && typeof (row as Record<string, unknown>).name === 'string' && typeof (row as Record<string, unknown>).type === 'string' && typeof (row as Record<string, unknown>).content === 'string');
    if (!rows(record.added) || !rows(record.kept) || typeof record.ipv4 !== 'string') return undefined;
    return record as unknown as SiteRecordsOutcome;
}

interface AddDomainModalProps {
    onClose: () => void;
    onSuccess: (outcome?: { siteRecords?: SiteRecordsOutcome }) => void;
    // The name the form opens with (alpha.115: the end of server setup offers
    // the main domain). Only a starting value: nothing is sent until the
    // person submits the form.
    initialDomain?: string;
}

// What this server can host right now drives which choices the dialog offers
// (lib/hostingCapabilities.ts). A website needs a web server, a DNS-only
// domain needs a local authoritative service. The requirement follows the ROLE
// the domain will play, not a fixed service list.
// Bu sunucunun şu anda neyi barındırabildiği, pencerenin hangi seçenekleri
// sunacağını belirler (lib/hostingCapabilities.ts). Web sitesi web sunucusu,
// yalnız-DNS alan adı yerel bir yetkili hizmet ister. Gereksinim, alan adının
// üstleneceği ROLÜ izler; sabit bir servis listesini değil.

// What the domain is FOR — the only question this screen asks (D-013).
// Runtime (PHP on/off, version, Node…) is a SITE SETTING chosen afterwards on
// the site's own page, not a creation-time decision: a person adding a domain
// knows their purpose, not their interpreter. "website" creates a site the
// panel serves; "dnsonly" creates just the zone. `static` and `php` are the
// same thing with the PHP switch off and on, so they stopped being separate
// choices here.
// Domain NE İÇİN — bu ekranın sorduğu tek soru (D-013). Runtime (PHP açık/kapalı,
// sürüm, Node…) sonradan sitenin kendi sayfasında seçilen bir SİTE AYARIdır,
// oluşturma anı kararı değil: domain ekleyen kişi amacını bilir, yorumlayıcısını
// değil. "website" panelin sunduğu bir site oluşturur; "dnsonly" yalnız zone.
// `static` ile `php` aynı şeyin PHP anahtarı kapalı/açık hâlidir; bu yüzden
// burada ayrı seçenek olmaktan çıktılar.
type Purpose = DomainPurpose;

const API_BASE = '/api/v1';

interface DomainCreatePartialSuccess {
    error: string;
    code: 'DNS_PUBLICATION_PENDING';
    partial_success: true;
    domain_id: number;
    domain: string;
    zone_exists: boolean;
    action?: string;
}

// A failed HTTP status does not always mean Create did nothing. When the site
// already exists but DNS publication failed, preserve the structured success
// identity before the generic API-error reader intentionally narrows the body.
async function readDomainCreatePartialSuccess(res: Response): Promise<DomainCreatePartialSuccess | null> {
    try {
        const value: unknown = await res.clone().json();
        if (!value || typeof value !== 'object') return null;
        const data = value as Record<string, unknown>;
        if (
            data.code !== 'DNS_PUBLICATION_PENDING' ||
            data.partial_success !== true ||
            typeof data.domain_id !== 'number' ||
            data.domain_id <= 0 ||
            typeof data.domain !== 'string' ||
            data.domain === ''
        ) {
            return null;
        }
        return {
            error: typeof data.error === 'string' ? data.error : '',
            code: 'DNS_PUBLICATION_PENDING',
            partial_success: true,
            domain_id: data.domain_id,
            domain: data.domain,
            zone_exists: data.zone_exists === true,
            action: typeof data.action === 'string' ? data.action : undefined,
        };
    } catch {
        return null;
    }
}

// The id of the site that was just created, from the create answer (its keys
// are the Go field names). null when the answer does not carry one.
function readCreatedDomainId(value: unknown): number | null {
    if (!value || typeof value !== 'object') return null;
    const id = (value as Record<string, unknown>).DomainID;
    return typeof id === 'number' && Number.isInteger(id) && id > 0 ? id : null;
}

// What the certificate request came to, in the order D-024 asks for: what
// happened to the certificate (the site exists in every case), who acts, the
// next action and how it resumes. A verified failure, a refusal because
// something is missing, and an unknown result are three different sentences.
function CertificateOutcome({ name, outcome }: { name: string; outcome: SiteCertificateOutcome }) {
    const { t } = useI18n();
    if (outcome.kind === 'failed') {
        const reason = outcome.error.reason ?? '';
        return (
            <div className="space-y-3" data-certificate-outcome={isIssueFailure(outcome.error) ? 'failed' : 'refused'}>
                <p className="max-w-[75ch] text-sm leading-relaxed text-fg">{t('domains.add.sslFailed', { name })}</p>
                {isIssueFailure(outcome.error) ? (
                    <div role="alert" className="rounded-lg border border-danger/30 bg-danger/10 p-3 text-sm leading-relaxed text-fg">
                        <p className="max-w-[75ch] break-words">
                            {t(ISSUE_FAILURE_KEYS[reason as keyof typeof ISSUE_FAILURE_KEYS] ?? ISSUE_FAILURE_KEYS.tool, { domain: outcome.error.vars?.domain ?? name })}
                        </p>
                        {outcome.error.vars?.detail && (
                            <p className="mt-1.5 max-w-[75ch] break-words text-fg-muted">
                                {t('ssl.issueFailure.said', { detail: outcome.error.vars.detail })}
                            </p>
                        )}
                    </div>
                ) : (
                    <ErrorBanner error={outcome.error} />
                )}
                <p className="max-w-[75ch] text-sm leading-relaxed text-fg-muted">{t('domains.add.sslFailedNext')}</p>
            </div>
        );
    }
    const key = {
        waitingForOwner: 'domains.add.sslWaitingForOwner',
        activationPending: 'domains.add.sslActivationPending',
        dependentsPending: 'domains.add.sslDependentsPending',
        unknown: 'domains.add.sslUnknown',
        issued: 'domains.add.sslIssued',
    }[outcome.kind] as Parameters<typeof t>[0];
    const unknown = outcome.kind === 'unknown';
    return (
        <p
            role={unknown ? 'alert' : 'status'}
            data-certificate-outcome={outcome.kind}
            className={`max-w-[75ch] rounded-lg border p-3 text-sm leading-relaxed text-fg ${unknown ? 'border-warning-mark/50 bg-warning-mark/20' : 'border-border bg-surface-2/50'}`}
        >
            {t(key, { name })}
        </p>
    );
}

export function AddDomainModal({ onClose, onSuccess, initialDomain = '' }: AddDomainModalProps) {
    const { t } = useI18n();
    const navigate = useNavigate();
    const [domainName, setDomainName] = useState(initialDomain);
    // The page under this dialog reads the same capabilities; the dialog uses
    // that answer and that request (lib/remote.ts) instead of asking again.
    // Bu pencerenin altındaki sayfa aynı yetenekleri okur; pencere yeniden
    // sormak yerine o yanıtı ve o isteği kullanır.
    const capabilities = useHostingCapabilities();
    const caps = capabilities.remote.state === 'known' ? capabilities.remote.value : null;
    // What the person chose, or nothing yet. Until they choose, the default is
    // what can actually work here: a website, unless the server is KNOWN to
    // have no web server, where only the DNS zone is possible. A choice made
    // while the answer was still on its way is theirs and is kept.
    // Kişinin seçtiği, ya da henüz hiçbir şey. Seçene kadar varsayılan burada
    // gerçekten çalışabilecek olandır: web sitesi; sunucuda web sunucusu
    // olmadığı BİLİNİYORSA yalnız DNS bölgesi. Yanıt gelmeden yapılan seçim
    // kişinindir ve korunur.
    const [chosenPurpose, setPurpose] = useState<Purpose | null>(null);
    const purpose: Purpose = chosenPurpose ?? (caps && caps.web_server === '' ? 'dnsonly' : 'website');
    const [sslEnabled, setSSLEnabled] = useState(false);
    const [loading, setLoading] = useState(false);
    // After the site exists and SSL was ticked, the dialog stays open for the
    // certificate request: `requesting` while its one request is on the way,
    // `done` once it has an outcome. The site is created in both; the form is
    // never shown again, so Create cannot be pressed twice.
    // Site oluştuktan ve SSL işaretlendikten sonra pencere sertifika isteği
    // için açık kalır; site her iki durumda da oluşmuştur, form bir daha
    // gösterilmez, böylece Oluştur iki kez basılamaz.
    const [certificate, setCertificate] = useState<
        | { phase: 'requesting'; name: string }
        | { phase: 'done'; name: string; outcome: SiteCertificateOutcome }
        | null
    >(null);
    const [siteRecords, setSiteRecords] = useState<SiteRecordsOutcome | undefined>(undefined);
    // The full contract object, not just text: a coded refusal may carry an
    // in-panel fix path that ErrorBanner turns into a button.
    // Yalnız metin değil sözleşme nesnesi: kodlu ret, ErrorBanner'ın düğmeye
    // çevirdiği panel-içi çözüm yolu taşıyabilir.
    const [error, setError] = useState<ApiError | null>(null);

    // Whether a domain of this purpose can be created, as one of four answers:
    // checking, could not check, open, or blocked with the half that is
    // missing. `blocked` exists only for a known answer (gateOn), so the
    // "choose a DNS engine" blocker cannot be drawn while the read is still
    // running or after it failed - which is what an owner saw for seconds on a
    // server that had DNS (reported 8 Oct 2026). Explicit external or remote
    // DNS permits websites without a local publisher; a DNS-only domain still
    // requires a proven local authoritative service (dnsBlocker).
    // Bu amaçla alan adı oluşturulabilir mi: kontrol ediliyor, kontrol
    // edilemedi, açık ya da eksik yarısıyla engelli. `blocked` yalnız bilinen
    // yanıt için vardır; bu yüzden "DNS motoru seç" engeli okuma sürerken ya
    // da başarısız olduktan sonra çizilemez. DNS'i olan bir sunucuda sahibin
    // saniyelerce gördüğü buydu (8 Eki 2026).
    const dns = gateOn(capabilities.remote, (value) => dnsBlocker(value, purpose));
    // A website needs a web server — and ONLY a web server. PHP is no longer a
    // precondition here (D-013): a site is created first, its PHP switch is a
    // setting afterwards, so a server without PHP can still host websites.
    // An option is shown as unavailable only once the answer is known.
    // Web sitesi bir web sunucusu ister — ve YALNIZ onu. PHP artık burada ön
    // koşul değildir (D-013): önce site oluşturulur, PHP anahtarı sonradan bir
    // ayardır; yani PHP'siz bir sunucu da web sitesi barındırabilir. Seçenek
    // ancak yanıt bilindiğinde kullanılamaz gösterilir.
    const websiteAvailable = caps ? hostingDNSReady(caps) && caps.web_server !== '' : true;
    const dnsOnlyAvailable = caps ? localDNSReady(caps) : true;

    const purposeOptions: {
        id: Purpose;
        icon: typeof Globe;
        available: boolean;
        requirement: string | null;
    }[] = [
        { id: 'website', icon: Server, available: websiteAvailable, requirement: !caps || dns.state === 'blocked' || websiteAvailable ? null : t('domains.add.needsWebServer') },
        { id: 'dnsonly', icon: Network, available: dnsOnlyAvailable, requirement: null },
    ];

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        // Nothing is sent unless this server is known to be able to publish it.
        // Sunucunun bunu yayımlayabildiği bilinmeden hiçbir şey gönderilmez.
        if (dns.state !== 'open') return;
        setLoading(true);
        setError(null);

        try {
            // A website is created as a plain site; PHP is off until the
            // operator turns it on in the site's settings (D-013). "static" is
            // that off state — the wire value stays the same, only the question
            // asked on screen changed.
            // Web sitesi düz bir site olarak oluşturulur; PHP, operatör sitenin
            // ayarlarından açana dek kapalıdır (D-013). "static" o kapalı
            // hâldir — kablo üzerindeki değer aynı kaldı, yalnız ekranda
            // sorulan soru değişti.
            const body: Record<string, unknown> = {
                domain: domainName,
                project_type: purpose === 'website' ? 'static' : 'dnsonly',
                ssl_type: purpose !== 'dnsonly' && sslEnabled ? 'letsencrypt' : 'none',
            };

            const res = await fetch(`${API_BASE}/domains/create`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            });

            if (!res.ok) {
                const partial = await readDomainCreatePartialSuccess(res);
                if (partial) {
                    // Creation is finished; never leave the form enabled for a
                    // duplicate retry. The parent closes this modal and refreshes
                    // the list, then we continue at the newly created domain.
                    const action = partial.action?.startsWith('/domains/')
                        ? partial.action
                        : `/domains/${encodeURIComponent(partial.domain)}`;
                    showToast('warning', t('domains.add.createdDnsPending', { name: partial.domain }));
                    // The certificate needs the name to resolve to this
                    // server, which the unfinished DNS setup has not done:
                    // say that it was not requested and where to do it.
                    // Sertifika, adın bu sunucuya çözülmesini ister; yarım
                    // DNS kurulumu bunu yapmadı: istenmediğini söyle.
                    if (body.ssl_type === 'letsencrypt') showToast('warning', t('domains.add.sslNotRequested', { name: partial.domain }));
                    onSuccess();
                    navigate(action);
                    return;
                }
                const apiErr = await readApiError(res);
                if (isSiteWebServerRefused(apiErr)) {
                    // The web server refused the site: a sentence with a
                    // command to run stays in the dialog, in the page's
                    // language, with nginx's own line under it; it is not
                    // a toast that leaves after five seconds.
                    setError(siteWebServerRefusedIn(apiErr, t));
                    return;
                }
                if (!apiErr.message && !apiErr.code) apiErr.message = t('domains.add.failed');
                setError(apiErr);
                showToast('error', apiErrorText(apiErr, t, 'domains.add.failed'));
                return;
            }

            showToast('success', t('domains.add.created', { name: domainName }));
            // Only read: what happened to the site's names, when the server says.
            let answer: unknown = null;
            try { answer = await res.json(); } catch { answer = null; }
            const records = readSiteRecords(answer);
            const createdId = readCreatedDomainId(answer);
            if (body.ssl_type !== 'letsencrypt') {
                onSuccess({ siteRecords: records });
                return;
            }
            setSiteRecords(records);
            if (createdId === null) {
                // The site exists but the answer does not name it, so the
                // certificate cannot be requested for it from here.
                setCertificate({ phase: 'done', name: domainName, outcome: { kind: 'unknown' } });
                return;
            }
            // The site exists. Ask once for its certificate and say what came
            // of it; the site stays created whatever the answer is.
            // Site var. Sertifikasını bir kez iste ve ne olduğunu söyle; yanıt
            // ne olursa olsun site oluşmuş kalır.
            setCertificate({ phase: 'requesting', name: domainName });
            const outcome = await requestSiteCertificate(createdId);
            if (outcome.kind === 'issued') {
                showToast('success', t('domains.add.sslIssued', { name: domainName }));
                onSuccess({ siteRecords: records });
                return;
            }
            setCertificate({ phase: 'done', name: domainName, outcome });
        } catch {
            const apiErr: ApiError = { message: t('domains.add.failed') };
            setError(apiErr);
            showToast('error', apiErr.message);
        } finally {
            setLoading(false);
        }
    };

    // Once the site is created and SSL was ticked, the same dialog turns into
    // the certificate request's own screen: what is happening, or what it came
    // to, who acts and what to do next. There is no way back to the form.
    // Site oluşup SSL işaretliyse aynı pencere sertifika isteğinin kendi
    // ekranına döner; forma geri dönüş yoktur.
    const finish = (openSSL: boolean) => {
        onSuccess({ siteRecords });
        if (openSSL && certificate) navigate(`/domains/${encodeURIComponent(certificate.name)}?tab=ssl`);
    };

    return (
        <Dialog
            id="add-domain"
            icon={certificate ? Lock : Globe}
            width="xl"
            title={certificate
                ? t(certificate.phase === 'requesting' ? 'domains.add.sslRequesting.title' : 'domains.add.sslOutcome.title', { name: certificate.name })
                : t('domains.add.title')}
            description={certificate ? t('domains.add.sslCreated', { name: certificate.name }) : t('domains.add.subtitle')}
            busy={certificate ? certificate.phase === 'requesting' : loading}
            onDismiss={certificate ? () => finish(false) : onClose}
            onSubmit={certificate ? undefined : handleSubmit}
            actions={
                certificate ? (
                    certificate.phase === 'requesting' ? (
                        <Button type="button" variant="primary" disabled>
                            {t('domains.add.sslRequesting.button')}
                        </Button>
                    ) : (
                        <>
                            <Button type="button" variant="secondary" onClick={() => finish(false)}>
                                {t('domains.add.sslDone')}
                            </Button>
                            <Button type="button" variant="primary" onClick={() => finish(true)}>
                                {t('domains.add.sslOpen')}
                            </Button>
                        </>
                    )
                ) : (
                    <>
                        <Button type="button" variant="secondary" onClick={onClose}>
                            {t('common.cancel')}
                        </Button>
                        <Button type="submit" variant="primary" disabled={loading || dns.state !== 'open'}>
                            {loading ? t('domains.add.creating') : t('domains.add.create')}
                        </Button>
                    </>
                )
            }
        >
            {certificate ? (
                certificate.phase === 'requesting' ? (
                    <div className="space-y-3">
                        <Checking label={t('domains.add.sslRequesting.wait', { name: certificate.name })} />
                        <p className="max-w-[75ch] text-sm leading-relaxed text-fg-muted">{t('domains.add.sslRequesting.body')}</p>
                    </div>
                ) : (
                    <CertificateOutcome name={certificate.name} outcome={certificate.outcome} />
                )
            ) : (
            <div className="space-y-4">
                <ErrorBanner error={error} />

                {/* The read failed: say that, keep what was typed, offer the
                    read again. Not the blocker - nothing is known to be missing.
                    Okuma başarısız: bunu söyle, yazılanı koru, okumayı yeniden
                    sun. Engel değil - eksik olduğu bilinen bir şey yok. */}
                {dns.state === 'unknown' && (
                    <CouldNotCheck
                        text={t('domains.add.dnsUnknown')}
                        onRetry={() => void capabilities.retry()}
                        busy={capabilities.reading}
                    />
                )}
                {dns.state === 'blocked' && (
                    <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-warning-mark/50 bg-warning-mark/20 p-4 text-sm text-fg">
                        {/*
                          Say which half is missing. An engine that is active but
                          has no identity must not be told to "activate BIND or
                          PowerDNS"; the button and the sentence name one fix.
                          Hangi yarının eksik olduğunu söyle. Etkin ama kimliksiz
                          bir motora "BIND ya da PowerDNS'i etkinleştir" denmemeli;
                          düğme ile cümle aynı tek düzeltmeyi adlandırır.
                        */}
                        <span>{dns.reason === 'identity' ? t('err.DNS_SETTINGS_REQUIRED') : t('domains.add.needsDns')}</span>
                        {/*
                          Both halves of "DNS is missing" are fixed in the same
                          place: the DNS infrastructure section installs and
                          activates an engine, and stages the identity. The
                          Services page cannot install a DNS engine any more
                          (DNS_ENGINE_WORKFLOW_REQUIRED), so sending a fresh
                          host there was a dead end (R-029, screen side).
                          "DNS eksik"in iki yarısı da aynı yerde düzelir: DNS
                          altyapısı bölümü motoru kurup etkinleştirir ve kimliği
                          hazırlar. Servisler sayfası artık DNS motoru kuramaz
                          (DNS_ENGINE_WORKFLOW_REQUIRED); taze sunucuyu oraya
                          göndermek çıkmaz sokaktı (R-029, ekran tarafı).
                        */}
                        <button
                            type='button'
                            onClick={() => navigate('/dns-infrastructure')}
                            className='rounded-lg bg-primary px-3 py-1.5 text-xs font-semibold text-primary-fg'
                        >
                            {dns.reason === 'identity'
                                ? t('err.DNS_SETTINGS_REQUIRED.action')
                                : t('err.DNS_SERVER_REQUIRED.action')}
                        </button>
                    </div>
                )}
                <div>
                    <label className="block text-sm font-medium text-fg-muted mb-2">
                        {t('domains.add.domainName')}
                    </label>
                    <input
                        type="text"
                        value={domainName}
                        onChange={(e) => setDomainName(e.target.value)}
                        className="w-full bg-surface-2 border border-border rounded-lg px-4 py-3 text-fg focus:border-primary"
                        placeholder="example.com"
                        required
                    />
                    <p className="text-xs text-fg-subtle mt-1">{t('domains.add.domainHint')}</p>
                </div>

                <div>
                    <label className="block text-sm font-medium text-fg-muted mb-2">
                        {t('domains.add.purpose')}
                    </label>
                    <div className="grid gap-2">
                        {purposeOptions.map(({ id, icon: Icon, available, requirement }) => (
                            <label
                                key={id}
                                className={`flex items-start gap-3 rounded-lg border p-3 transition-colors ${
                                    purpose === id
                                        ? 'border-primary bg-primary/5'
                                        : 'border-border bg-surface-2/50'
                                } ${available ? 'cursor-pointer hover:border-primary/50' : 'opacity-60'}`}
                            >
                                <input
                                    type="radio"
                                    name="purpose"
                                    checked={purpose === id}
                                    disabled={!available}
                                    onChange={() => setPurpose(id)}
                                    className="mt-1"
                                />
                                <Icon className="mt-0.5 h-4 w-4 shrink-0 text-fg-subtle" />
                                <div className="min-w-0">
                                    <div className="text-sm font-medium text-fg">{t(`domains.add.purpose.${id}` as Parameters<typeof t>[0])}</div>
                                    <p className="text-xs text-fg-subtle">{t(`domains.add.purpose.${id}.desc` as Parameters<typeof t>[0])}</p>
                                    {requirement && (
                                        <p className="mt-1 text-xs font-medium text-warning">{requirement}</p>
                                    )}
                                </div>
                            </label>
                        ))}
                    </div>
                </div>

                {/* The PHP version picker used to live here. It moved to the
                    site's own settings (D-013): choosing an interpreter is
                    not part of adding a domain, and advertising PHP on a
                    server that has none contradicted "what isn't installed
                    is invisible".
                    PHP sürüm seçici burada dururdu. Sitenin kendi ayarlarına
                    taşındı (D-013): yorumlayıcı seçmek domain eklemenin
                    parçası değildir ve PHP'si olmayan bir sunucuda PHP'yi
                    reklam etmek "kurulu olmayan görünmez" ilkesine aykırıydı. */}

                {purpose !== 'dnsonly' && (
                    <div className="flex items-start gap-3 p-4 bg-surface-2/50 rounded-lg">
                        <input
                            type="checkbox"
                            id="ssl"
                            checked={sslEnabled}
                            onChange={(e) => setSSLEnabled(e.target.checked)}
                            className="mt-1"
                        />
                        <div className="flex-1">
                            <label htmlFor="ssl" className="flex items-center gap-2 text-sm font-medium text-fg-muted cursor-pointer">
                                <Lock className="w-4 h-4" />
                                {t('domains.add.ssl')}
                            </label>
                            <p className="text-xs text-fg-subtle mt-1">{t('domains.add.sslHint')}</p>
                        </div>
                    </div>
                )}

                {/* One status line with a height of its own: the checking line
                    while the answer is on its way, then what is known. The
                    form above it does not move when the answer arrives.
                    Kendi yüksekliği olan tek durum satırı: yanıt yoldayken
                    kontrol satırı, sonra bilinen. Yanıt gelince üstündeki form
                    yerinden oynamaz. */}
                <div className="min-h-5">
                    {dns.state === 'checking' && <Checking label={t('dns.checkingServer')} />}
                    {caps && caps.dns_server !== '' && (
                        <p className="text-xs leading-5 text-fg-subtle">
                            {t('domains.add.dnsServed', { server: caps.dns_server })}
                        </p>
                    )}
                </div>
            </div>
            )}
        </Dialog>
    );
}
