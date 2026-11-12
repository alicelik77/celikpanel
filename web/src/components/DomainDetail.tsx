import { Fragment, Suspense, lazy, useCallback, useState, useEffect, type ComponentType, type LazyExoticComponent, type ReactNode } from 'react';
import { replaceCurrentRouterURL, useLocation, useSearchParams } from '../router';
import { publishGuideTopic } from '../help/guide/context';
import { ArrowLeft, Globe, Lock, ExternalLink } from 'lucide-react';
import { decodeSSLData, type SSLRuntimeSummary } from './DomainSSLSettings';
import { DomainConnection } from './DomainConnection';
import { useI18n } from '../i18n';
import { formatTime } from '../lib/dateFormat';
import { Button, Checking, CouldNotCheck, KnownEmpty, Spinner } from './ui';
import { Guidance } from './kit/rows/Guidance';
import { StateLabel } from './kit/rows/StateLabel';
import { siteKindWord, sizeWord } from './kit/words';
import { confirmLeaveIfDirty } from './kit/save/leaveGuard';
import { useAuth } from '../auth/AuthContext';
import { useHostingCapabilities } from '../lib/hostingCapabilities';
import { decodeList, lastKnown, useRemote, useRereadWhileRechecking } from '../lib/remote';
import {
    hasAnyDomainAccess,
    hasDomainAccess,
    normalizeDomainAccess,
    type DomainAccess,
    type DomainCapability,
} from '../auth/domainAccess';
import {
    buildSections,
    guideTopic,
    isOpenPlace,
    legacyPlace,
    requestedPlace,
    withPlace,
    type Place,
    type SectionId,
} from './domain/sections';
import { SectionRow, SECTION_PANEL_ID, sectionTabId } from './domain/SectionRow';
import type { DomainSectionProps } from './domain/SectionParts';

// The nine sections of the page (alpha.132), each in a file of its own and
// fetched when it is opened: the page's own bundle stays the shell (the row,
// the header, the facts) and the screens of a section load with it. The page
// bundle had less than 1 KiB under its limit, and the limit is not raised.
// Sayfanın dokuz bölümü, her biri kendi dosyasında ve açılınca getirilir;
// sayfa paketi yalnız kabuktur. Sınır yükseltilmez.
const SECTION_VIEWS: Record<SectionId, LazyExoticComponent<ComponentType<DomainSectionProps>>> = {
    overview: lazy(() => import('./domain/sections/OverviewSection').then((module) => ({ default: module.OverviewSection }))),
    site: lazy(() => import('./domain/sections/SiteSection').then((module) => ({ default: module.SiteSection }))),
    files: lazy(() => import('./domain/sections/FilesSection').then((module) => ({ default: module.FilesSection }))),
    mail: lazy(() => import('./domain/sections/MailSection').then((module) => ({ default: module.MailSection }))),
    databases: lazy(() => import('./domain/sections/DatabasesSection').then((module) => ({ default: module.DatabasesSection }))),
    dns: lazy(() => import('./domain/sections/DnsSection').then((module) => ({ default: module.DnsSection }))),
    security: lazy(() => import('./domain/sections/SecuritySection').then((module) => ({ default: module.SecuritySection }))),
    backups: lazy(() => import('./domain/sections/BackupsSection').then((module) => ({ default: module.BackupsSection }))),
    advanced: lazy(() => import('./domain/sections/AdvancedSection').then((module) => ({ default: module.AdvancedSection }))),
};
// A section the plan does not include opens this one-block page instead.
// Planın içermediği bölüm bunun yerine bu tek bloklu sayfayı açar.
const NotInPlanSection = lazy(() => import('./domain/sections/NotInPlanSection').then((module) => ({
    default: module.NotInPlanSection,
})));
// The site's configuration file notice (D-031), for an administrator.
// Sitenin yapılandırma dosyası bildirimi; gerektiğinde getirilir.
const SiteConfigNotice = lazy(() => import('./DomainSiteConfig').then((module) => ({
    default: module.SiteConfigNotice,
})));
// alpha.128 ops (G22): the health strip, with its own copy, in its own chunk.
const DomainHealthStrip = lazy(() => import('./DomainHealthStrip').then((module) => ({
    default: module.DomainHealthStrip,
})));

interface Domain {
    id: number;
    domain_name: string;
    php_version?: string;
    project_type?: string;
    ssl_enabled?: boolean;
    status: string;
    created_at: string;
    disk_usage?: number;
    bandwidth?: number;
    access?: DomainAccess;
    site_config?: { state: string; kept_by_choice?: boolean; pending_reason?: string; kind?: string };
    /** The permissions the subscription's rights close (alpha.117, D-042); absent when none. */
    rights_closed?: string[];
}

// The measured usage of one domain. An answer without the numbers is not
// "0 B used".
// Bir alan adının ölçülen kullanımı. Sayıları taşımayan yanıt "0 B" değildir.
interface DomainUsage {
    disk_usage: number;
    bandwidth: number;
    /** alpha.118 (D-044): whether this month's traffic is counted. */
    traffic_state?: string;
    /** alpha.127: the disk is the last measurement; a new one runs behind the page. */
    rechecking?: boolean;
}
function decodeUsage(raw: unknown): DomainUsage {
    const body = raw as { disk_usage?: unknown; bandwidth?: unknown; traffic_state?: unknown; rechecking?: unknown } | null;
    if (!body || typeof body.disk_usage !== 'number' || typeof body.bandwidth !== 'number') throw new Error('shape');
    return { disk_usage: body.disk_usage, bandwidth: body.bandwidth, traffic_state: typeof body.traffic_state === 'string' ? body.traffic_state : undefined, rechecking: body.rechecking === true };
}

// What became of looking this domain up in the list the server gave.
// `absent` and `noAccess` are answers; a list that could not be read is
// neither, and none of the three sends the person back to the list unasked.
// Bu alan adının, sunucunun verdiği listede aranmasının sonucu. `absent` ve
// `noAccess` yanıttır; okunamayan liste ikisi de değildir ve hiçbiri kişiyi
// sormadan listeye geri göndermez.
const LIST_FRESH_FOR_MS = 30_000;

type Lookup =
    | { state: 'found'; domain: Domain }
    | { state: 'absent' }
    | { state: 'noAccess'; name: string };

function lookUp(rows: unknown[], domainId: number, isTeamMember: boolean): Lookup {
    const found = rows.find((item) => (
        item !== null
        && typeof item === 'object'
        && !Array.isArray(item)
        && Number((item as { id?: unknown }).id) === domainId
    ));
    if (!found || typeof found !== 'object' || Array.isArray(found)) return { state: 'absent' };
    if (!isTeamMember) return { state: 'found', domain: found as Domain };

    const row = found as Domain & { access?: unknown };
    const access = normalizeDomainAccess(row.access);
    if (!access || !hasAnyDomainAccess(access)) return { state: 'noAccess', name: String(row.domain_name ?? '') };
    return { state: 'found', domain: { ...row, access } };
}

// The page of one domain as the address names it. The name is looked up in the
// list the server gives; that is the address the page below, the Domains page
// and the navigation read too, so one request serves all of them. A list that
// could not be read is said so with Retry, a name the server does not list is
// said so with the way back, and neither moves the person off this address.
//
// Adresin adlandırdığı alan adının sayfası. Ad, sunucunun verdiği listede
// aranır; aşağıdaki sayfa, Alan Adları sayfası ve gezinme de aynı adresi okur,
// tek istek hepsine yeter. Okunamayan liste Tekrar dene ile, sunucunun
// listelemediği ad geri dönüş yoluyla söylenir; ikisi de kişiyi bu adresten
// almaz.
export function DomainDetailByName({ domainName, onBack }: { domainName: string; onBack: () => void }) {
    const { t } = useI18n();
    const list = useRemote('/api/v1/domains', decodeList<unknown>);
    const listed = lastKnown(list.remote);
    const back = <Button type="button" icon={ArrowLeft} onClick={onBack}>{t('nav.domains')}</Button>;

    if (!listed) {
        return (
            <div className="p-6 md:p-8">
                {list.remote.state === 'loading' ? (
                    <Checking label={t('domain.checking')} />
                ) : (
                    <>
                        <CouldNotCheck text={t('domain.unknown')} onRetry={() => void list.retry()} busy={list.reading} />
                        <div className="mt-3">{back}</div>
                    </>
                )}
            </div>
        );
    }

    const row = listed.value.find((item) => (
        !!item && typeof item === 'object' && (item as { domain_name?: unknown }).domain_name === domainName
    )) as { id?: unknown } | undefined;
    const domainId = Number(row?.id);
    if (!row || !Number.isFinite(domainId)) {
        return (
            <div className="p-6 md:p-8">
                <KnownEmpty of={listed} icon={Globe} title={t('domain.absent')} hint={t('domain.absentHint')} action={back} />
            </div>
        );
    }

    return <DomainDetail key={domainId} domainId={domainId} onBack={onBack} />;
}

interface DomainDetailProps {
    domainId: number;
    onBack: () => void;
}


// A domain's page (alpha.132): the header with a quiet line of facts, one row
// of nine sections named by what the person wants to do, and the open
// section below. Which sections and parts are drawn is decided in one place
// (domain/sections.ts); each section's screens are in its own lazy file. The
// place on the page is in the address, so a reload, Back and a shared link
// keep it; every older ?tab= link still leads to the same screen.
//
// Alan adı sayfası: başlık ve tek satırlık bilgiler, işe göre adlandırılmış
// dokuz bölümlük tek satır ve altında açık bölüm. Hangi bölümün çizileceğine
// tek yerde karar verilir; sayfadaki yer adreste durur, eski ?tab=
// bağlantıları aynı ekrana götürür.
export function DomainDetail({ domainId, onBack }: DomainDetailProps) {
    const { t, locale } = useI18n();
    const { role } = useAuth();
    const isTeamMember = role === 'additional_user';
    const isAdmin = role === 'admin';
    const [searchParams, setSearchParams] = useSearchParams();
    const { pathname, hash } = useLocation();
    const requested = requestedPlace(searchParams);
    // The list is the same address the Domains page and the navigation read,
    // so this page shares their request. What this page changed itself (the
    // PHP version, whether a certificate is active) lies over the row until
    // the list is read again.
    // Liste, Alan Adları sayfasının ve gezinmenin okuduğu adrestir; bu sayfa
    // onların isteğini paylaşır. Sayfanın kendi değiştirdiği (PHP sürümü,
    // sertifikanın etkinliği), liste yeniden okunana dek satırın üzerinde durur.
    // The lookup by name just above has read it a moment ago; that answer is
    // used as it is and nothing is requested a second time.
    // Hemen üstteki ada göre arama onu az önce okudu; o yanıt olduğu gibi
    // kullanılır ve ikinci kez istek gönderilmez.
    const list = useRemote('/api/v1/domains', decodeList<unknown>, { freshFor: LIST_FRESH_FOR_MS });
    const [changed, setChanged] = useState<Partial<Domain>>({});

    const handleCertificateChange = useCallback((status: SSLRuntimeSummary) => {
        setChanged((current) => ({ ...current, ssl_enabled: status.activated }));
    }, []);
    const handlePhpVersionChange = useCallback((version: string) => {
        setChanged((current) => ({ ...current, php_version: version }));
    }, []);
    // The Configuration file page ended a certificate reason: the line above
    // the sections and the list's badge are read from the list, so read it again.
    const rereadList = useCallback(() => void list.retry(), [list]);

    const listed = lastKnown(list.remote);
    const lookup = listed ? lookUp(listed.value, domainId, isTeamMember) : null;
    const domain: Domain | null = lookup?.state === 'found' ? { ...lookup.domain, ...changed } : null;

    // The measured usage is read after the page is drawn: one domain, one
    // measurement. Until it answers, and if it cannot be read, the figures the
    // list carried stay; they are the server's own, only older.
    // Ölçülen kullanım sayfa çizildikten sonra okunur. Yanıt gelene dek ve
    // okunamazsa listenin taşıdığı rakamlar kalır; onlar da sunucunundur.
    const domainLoaded = domain !== null;
    const canViewStatistics = !isTeamMember
        || Boolean(domain?.access && hasDomainAccess(domain.access, 'statistics'));
    const usage = useRemote(domainLoaded && canViewStatistics ? `/api/v1/domains/${domainId}/usage` : null, decodeUsage);
    const measured = domainLoaded && canViewStatistics && usage.remote.state === 'known' ? usage.remote.value : null;
    useRereadWhileRechecking(measured?.rechecking === true, usage.remote.state === 'known' ? usage.remote.observedAt : undefined, () => void usage.retry());

    // The certificate line of the strip under the title reads the certificate
    // itself, at the address and with the decoder the overview card and the
    // certificate part of Security use, so the three share one request and
    // cannot disagree (10 Oct 2026). It is one of three things: being checked,
    // could not be checked (with the read again), or what the server said.
    // After a refresh that failed the earlier answer stays; the card and the
    // certificate part say that it is the earlier one.
    // Başlığın altındaki şeridin sertifika satırı, sertifikayı kendisi okur;
    // genel bakış kartıyla ve Güvenlik'in sertifika parçasıyla tek isteği
    // paylaşır. Üç şeyden biridir: kontrol ediliyor, kontrol edilemedi
    // (yeniden okumayla) ya da sunucunun söylediği.
    const showsCertificate = domain !== null
        && (domain.project_type || 'php') !== 'dnsonly'
        && (!isTeamMember || Boolean(domain.access && hasDomainAccess(domain.access, 'ssl')));
    const ssl = useRemote(showsCertificate ? `/api/v1/domains/${domainId}/ssl` : null, decodeSSLData);
    const certificate = showsCertificate ? lastKnown(ssl.remote) : undefined;

    // What the server can actually do — sections for services that are not
    // installed would be settings pages for ghosts. One shared read
    // (lib/hostingCapabilities.ts); the screens of a section use the same
    // answer. `caps` is the KNOWN answer or null, and the three states mean:
    //   - being checked:      every section stays; nothing is hidden on a guess;
    //   - could not be checked: every section stays; the screen that needs the
    //                         answer says so itself and offers the read again;
    //   - known:              only now is a section for a missing service removed.
    // A team member is never asked: the server-wide inventory is not theirs.
    // Sunucunun gerçekten yapabildiği. Kontrol edilirken ve kontrol
    // edilemediğinde her bölüm kalır; ancak bilindiğinde eksik servisin bölümü
    // kaldırılır. Ekip üyesi için hiç sorulmaz.
    const capabilities = useHostingCapabilities({ enabled: !isTeamMember });
    const caps = !isTeamMember && capabilities.remote.state === 'known' ? capabilities.remote.value : null;
    // Which sections exist follows the last answer the server gave. A refresh
    // that failed is not a reason to bring back a section the server ruled
    // out, nor to take one away.
    // Hangi bölümlerin var olduğu, sunucunun verdiği son yanıtı izler.
    const sectionCaps = isTeamMember ? null : lastKnown(capabilities.remote)?.value ?? null;

    // The sections of this domain for this viewer (domain/sections.ts): its
    // role, its grants as a team member, the plan's closed rights (D-042) and
    // the server's engines.
    // Bu izleyici için bu alan adının bölümleri.
    const sections = domain ? buildSections({
        role,
        projectType: domain.project_type || 'php',
        closedRights: domain.rights_closed ?? [],
        access: isTeamMember ? domain.access : undefined,
        server: sectionCaps ? { mail: sectionCaps.mail_server, databases: sectionCaps.database_servers.length > 0 } : null,
    }) : [];
    // The section the address asks for when it is drawn, otherwise the first.
    // Adresin istediği bölüm çiziliyse o, değilse ilki.
    const current = (requested && sections.find((section) => section.id === requested.section)) || sections[0];
    const shownPart = current && requested?.section === current.id && requested.part && current.parts.some((part) => part.id === requested.part)
        ? requested.part
        : null;
    // The settled answer: a team member's sections never wait for the server;
    // everyone else's wait for its engines before one is ruled out.
    const settled = isTeamMember || sectionCaps !== null;

    // The guide's "?" opens the chapter of the section (or part) on screen.
    // Kılavuzun "?" düğmesi ekrandaki bölümün bölümünü açar.
    const topic = current ? guideTopic(current.id, shownPart) : null;
    useEffect(() => publishGuideTopic(topic), [topic]);

    // The address says where the person is. An older ?tab= link, or a place
    // that is not on this page any more, is written over (replacing the
    // entry, so Back does not return to it) once the page knows what it has.
    // The person is moved only off a section that is not there; one that
    // comes back later (a service installed since) does not pull them to it.
    // Adres kişinin nerede olduğunu söyler. Eski ?tab= bağlantısı ya da artık
    // sayfada olmayan yer, sayfa ne olduğunu bilince değiştirilir.
    const canonical = current
        && (searchParams.has('tab') || searchParams.has('section') || searchParams.has('part'))
        && (settled || !requested || requested.section === current.id)
        ? withPlace(searchParams, { section: current.id, part: shownPart ?? undefined }).toString()
        : null;
    useEffect(() => {
        if (canonical === null || canonical === searchParams.toString()) return;
        setSearchParams(new URLSearchParams(canonical), { replace: true });
    }, [canonical, searchParams, setSearchParams]);

    // Moving to a section, or to a part of one. Every switch of section (the
    // row, the Overview's tiles, the health strip's fixes, the configuration
    // file notice) passes through here and nowhere else: what must hold the
    // person on the section they are leaving is asked at this one place.
    //   - Another section: its screens are put away, so unsaved changes ask
    //     first in the panel's own dialog (kit/save/leaveGuard). "Leave" gives
    //     them up, so the router does not ask a second time; "Stay" keeps the
    //     person and the changes where they are.
    //   - Another part of the open section: every part stays drawn on the
    //     same page (SectionParts), nothing is put away and nothing is asked;
    //     the address follows in place.
    // Bir bölüme ya da bölümün bir parçasına geçiş; bölüm değiştiren her yol
    // yalnız buradan geçer. Başka bölüme geçerken kaydedilmemiş değişiklik
    // panelin penceresiyle bir kez sorulur; aynı bölümün başka parçası aynı
    // sayfada çizili kaldığından hiçbir şey sorulmaz.
    const go = async (place: Place, options: { replace?: boolean } = {}) => {
        if (current && place.section === current.id && (place.part ?? null) === shownPart && !options.replace) return;
        const next = withPlace(searchParams, place);
        if (current && place.section === current.id) {
            const query = next.toString();
            if (replaceCurrentRouterURL(`${pathname}${query ? `?${query}` : ''}${hash}`)) return;
        }
        if (current && place.section !== current.id && !(await confirmLeaveIfDirty())) return;
        setSearchParams(next, { replace: options.replace });
    };

    if (list.remote.state === 'loading') {
        return (
            <div className="p-4 sm:p-6 md:p-8">
                <Checking label={t('domain.checking')} />
            </div>
        );
    }
    if (!domain) {
        // Not shown, and said why, with the way back as a choice.
        // Gösterilmez, nedeni söylenir ve geri dönüş bir seçenek olarak sunulur.
        const back = <Button type="button" icon={ArrowLeft} onClick={onBack}>{t('nav.domains')}</Button>;
        return (
            <div className="p-4 sm:p-6 md:p-8">
                {!listed || !lookup ? (
                    <>
                        <CouldNotCheck text={t('domain.unknown')} onRetry={() => void list.retry()} busy={list.reading} />
                        <div className="mt-3">{back}</div>
                    </>
                ) : (
                    <KnownEmpty
                        of={listed}
                        icon={Globe}
                        title={t(lookup.state === 'noAccess' ? 'domain.noAccess' : 'domain.absent')}
                        hint={t(lookup.state === 'noAccess' ? 'domain.noAccessHint' : 'domain.absentHint')}
                        action={back}
                    />
                )}
            </div>
        );
    }
    if (!current) return null;

    const projectType = domain.project_type || 'php';
    const isDnsOnly = projectType === 'dnsonly';
    const sslUsable = certificate?.value.certificate?.usable === true;
    const sslConfigured = certificate?.value.certificate?.activated === true;
    const canView = (capability: DomainCapability) => (
        !isTeamMember || Boolean(domain.access && hasDomainAccess(domain.access, capability))
    );
    // What the subscription's rights close (alpha.117, D-042). The server
    // refuses all of it anyway; the page does not offer what it would refuse.
    // Aboneliğin haklarının kapattığı; sunucu zaten hepsini reddeder.
    const closed = new Set(domain.rights_closed ?? []);
    const open = (permission: string) => !closed.has(permission);
    const View = SECTION_VIEWS[current.id];
    const configShown = current.id === 'advanced' && current.parts.some((part) => part.id === 'config');

    const facts: Array<{ key: string; content: ReactNode }> = [];
    if (!isTeamMember || canView('files') || canView('php')) {
        facts.push({
            key: 'type',
            content: <Fact label={t('domain.info.type')}>{siteKindWord(t, domain.project_type, { admin: isAdmin })}</Fact>,
        });
    }
    if (projectType === 'php' && canView('php')) {
        facts.push({
            key: 'php',
            content: <Fact label={t('domain.info.php')}>{domain.php_version || '—'}</Fact>,
        });
    }
    if (!isDnsOnly && canView('ssl')) {
        facts.push({
            key: 'ssl',
            content: (
                <Fact label={t('domain.info.ssl')}>
                    {certificate ? (
                        <span className={sslUsable ? 'text-success' : sslConfigured ? 'text-warning' : 'text-fg-subtle'}>
                            {sslUsable
                                ? t('domain.info.on')
                                : sslConfigured
                                  ? t('domain.info.sslIssue')
                                  : t('domain.info.off')}
                        </span>
                    ) : ssl.remote.state === 'unknown' ? (
                        // Not "off" and not "checking": the read failed. The
                        // way to read again is beside the words, because in
                        // most sections nothing else on the page offers it.
                        // "Kapalı" da değil "kontrol ediliyor" da: okuma
                        // başarısız. Yeniden okuma sözün yanındadır.
                        <span className="inline-flex flex-wrap items-baseline gap-x-2" data-ssl-fact="unknown">
                            <span>{t('domain.info.sslUnknown')}</span>
                            <button
                                type="button"
                                onClick={() => void ssl.retry()}
                                disabled={ssl.reading}
                                className="rounded font-medium text-primary underline underline-offset-2 hover:text-primary-hover disabled:cursor-progress disabled:opacity-60"
                            >
                                {t('common.retry')}
                            </button>
                        </span>
                    ) : (
                        <span className="text-fg-subtle" data-ssl-fact="checking">{t('domain.overview.ssl.checking')}</span>
                    )}
                </Fact>
            ),
        });
    }
    if (!isDnsOnly && canView('statistics')) {
        facts.push({
            key: 'disk',
            content: <Fact label={t('domain.info.disk')}>{sizeWord(measured ? measured.disk_usage : domain.disk_usage, locale)}</Fact>,
        });
        facts.push({
            key: 'traffic',
            content: (
                <Fact label={t('domain.info.traffic')}>
                    {measured?.traffic_state === 'unknown'
                        ? t('domain.info.trafficUnknown')
                        : t('domain.info.trafficMonth', { bytes: sizeWord(measured ? measured.bandwidth : domain.bandwidth, locale) })}
                </Fact>
            ),
        });
    }

    return (
        <div className="min-w-0 p-4 sm:p-6 md:p-8">
            {/* The list could not be read again: the page stays, and says that
                what it shows about this domain is the earlier answer.
                Liste yeniden okunamadı: sayfa kalır ve bu alan adı hakkında
                gösterdiğinin önceki yanıt olduğunu söyler. */}
            {list.remote.state === 'unknown' && listed && (
                <CouldNotCheck
                    className="mb-4"
                    text={t('common.staleNotice', {
                        time: formatTime(new Date(listed.observedAt), locale),
                    })}
                    onRetry={() => void list.retry()}
                    busy={list.reading}
                />
            )}
            {isAdmin && !isDnsOnly && domain.site_config && !configShown && (
                <Suspense fallback={null}>
                    <SiteConfigNotice
                        state={domain.site_config.state}
                        keptByChoice={domain.site_config.kept_by_choice}
                        pendingReason={domain.site_config.pending_reason}
                        kind={domain.site_config.kind}
                        onOpen={() => void go({ section: 'advanced', part: 'config' })}
                    />
                </Suspense>
            )}
            {/* Header */}
            <div className="mb-5">
                <button onClick={onBack} className="mb-3 inline-flex items-center gap-1.5 text-sm text-fg-muted hover:text-fg">
                    <ArrowLeft className="h-4 w-4" />
                    {t('nav.domains')}
                </button>
                <div className="flex flex-wrap items-center gap-3">
                    <span className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
                        {canView('ssl') && sslUsable ? <Lock className="h-5 w-5" /> : <Globe className="h-5 w-5" />}
                    </span>
                    <h1 className="min-w-0 break-all text-2xl font-bold tracking-tight">{domain.domain_name}</h1>
                    <StateLabel status={domain.status} admin={isAdmin} className="text-sm text-fg-muted" />
                    {!isDnsOnly && canView('files') && (
                        <a
                            href={`${sslUsable ? 'https' : 'http'}://${domain.domain_name}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="ml-auto inline-flex items-center gap-1.5 rounded-lg border border-border-strong bg-surface px-3 py-1.5 text-sm font-medium text-fg hover:bg-surface-2"
                        >
                            <ExternalLink className="h-4 w-4" />
                            {t('domain.openSite')}
                        </a>
                    )}
                </div>

                {/* Fact strip — status already lives next to the title, so
                    only the facts that add something. / Bilgi şeridi — durum
                    zaten başlığın yanında; yalnız bir şey katan bilgiler. */}
                {facts.length > 0 && (
                    <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-sm">
                        {facts.map((fact, index) => (
                            <Fragment key={fact.key}>
                                {index > 0 && <FactDivider />}
                                {fact.content}
                            </Fragment>
                        ))}
                    </div>
                )}
            </div>

            <SectionRow sections={sections} active={current.id} onSelect={(id) => void go({ section: id })} />

            <div id={SECTION_PANEL_ID} role="tabpanel" aria-labelledby={sectionTabId(current.id)} className="min-w-0">
                {/* The connection check leads the Overview: it is the
                    precondition for everything below it (operator, 25 Jul).
                    Bağlantı kontrolü Genel Bakış'ın başındadır. */}
                {current.id === 'overview' && (
                    <div className="mb-4">
                        <DomainConnection domainId={domain.id} domainName={domain.domain_name} />
                    </div>
                )}

                {/* alpha.128 ops (G22): the health strip leads the Overview;
                    a team member, who has no Overview, sees it in the first
                    section, with only the checks of its own grants. A fix
                    names an older tab; it leads to the section that holds it
                    now, or, when that is not open here, to whom to ask.
                    Sağlık şeridi Genel Bakış'ta; ekip üyesi ilk bölümde görür. */}
                {(current.id === 'overview' || (isTeamMember && current.id === sections[0]?.id)) && (
                    <Suspense fallback={null}>
                        <DomainHealthStrip
                            domainId={domain.id}
                            showsConnection={current.id === 'overview'}
                            hasTab={(tab, sub) => {
                                const place = legacyPlace(tab, sub);
                                return place !== null && isOpenPlace(sections, place);
                            }}
                            onGo={(tab, sub) => {
                                const place = legacyPlace(tab, sub);
                                if (place) void go(place);
                            }}
                        />
                    </Suspense>
                )}

                {/* Add domain created the site but could not publish its DNS
                    (DNS_PUBLICATION_PENDING): the page it opened says what
                    needs attention, who acts, what to do and how it resumes,
                    not only the toast that sent the person here (alpha.114).
                    Site olustu ama DNS yayimlanamadi: sayfa dikkat gerekeni soyler. */}
                {current.id === 'dns' && searchParams.get('attention') === 'dns-publication' && (
                    <Guidance
                        tone="waiting"
                        titleAs="h2"
                        className="mb-4"
                        data-dns-publication-attention=""
                        title={t('domains.attention.dnsTitle', { name: domain.domain_name })}
                        why={t('domains.attention.dnsWhy')}
                        who={t('domains.attention.dnsWho')}
                        next={t('domains.attention.dnsNext')}
                        then={t('domains.attention.dnsThen')}
                    />
                )}

                <Suspense fallback={<Spinner />}>
                    {current.notInPlan ? (
                        <NotInPlanSection key={current.id} section={current.id} rights={current.notInPlan} role={role} />
                    ) : (
                        <View
                            key={current.id}
                            domain={domain}
                            parts={current.parts}
                            part={shownPart}
                            sections={sections}
                            role={role}
                            isTeamMember={isTeamMember}
                            open={open}
                            canView={canView}
                            mailAvailable={caps?.mail_server ?? null}
                            go={go}
                            onCertificateChange={handleCertificateChange}
                            onPhpVersionChange={handlePhpVersionChange}
                            onReasonEnded={rereadList}
                        />
                    )}
                </Suspense>
            </div>
        </div>
    );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
    return (
        <span className="inline-flex items-baseline gap-1.5">
            <span className="text-fg-subtle">{label}</span>
            <span className="font-medium text-fg">{children}</span>
        </span>
    );
}

function FactDivider() {
    return <span aria-hidden className="h-3.5 w-px self-center bg-border-strong" />;
}

