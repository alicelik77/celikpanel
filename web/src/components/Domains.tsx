import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from '../router';
import { Globe, Plus, Trash2, ExternalLink, Settings, Lock, HardDrive, Network } from 'lucide-react';
import { RowActions, type RowAction } from './kit/rows/RowActions';
import { StateLabel } from './kit/rows/StateLabel';
import { siteKindWord, sizeWord } from './kit/words';
import { AddDomainModal, type SiteRecordsOutcome } from './AddDomainModal';
import { KeptSiteRecords } from './KeptSiteRecords';
import { confirmAction, followOperation, type OperationEnd, type OperationReport, type OperationStep } from '../lib/confirmAction';
import { useI18n } from '../i18n';
import { Button, CouldNotCheck, ErrorBanner, KnownEmpty, RemoteGate, SearchInput, Spinner, UsageBar } from './ui';
import { PageHeader } from './PageHeader';
import { BlockingProblems } from './BlockingProblems';
import { readApiError, refusalText, type ApiError } from '../lib/apiError';
import { DNSCheckToolNotice } from './DNSCheckToolNotice';
import { DeletionWaitingGuide } from './DeletionWaitingGuide';
import { deletionGuideFollowUp, deletionGuideText, deletionGuideWords, type DeletionGuideInput, type DeletionGuidePlace } from '../lib/deletionGuide';
import { Guidance } from './kit/rows/Guidance';
import { domainDeletionBlocksRetry, domainDeletionDetailKey, domainDeletionGuideReason, domainDeletionQuiet, domainDeletionReasonAutomatic, domainDeletionReasonKey, readDomainDeletionDKIM, readDomainDeletionOutcome, readDomainDeletionStage, readSavedDomainDeletion, readSavedDomainDeletionWithStage, type DomainDeletionFollowUp } from '../lib/domainDeletionPending';
import { decodeList, gateOn, invalidate, lastKnown, useRemote } from '../lib/remote';
import { SUBSCRIPTIONS_URL, decodeSubscriptions } from '../lib/subscriptions';
import { INFRASTRUCTURE_ZONES_URL, decodeInfrastructureZones, infrastructureZonePath } from '../lib/infrastructureZones';
import { dnsBlocker, dnsEngineBlockerText, useHostingCapabilities } from '../lib/hostingCapabilities';
import type { TranslationKey } from '../i18n/en';
import { useAuth } from '../auth/AuthContext';
import {
    hasAnyDomainAccess,
    hasDomainAccess,
    normalizeDomainAccess,
    type DomainAccess,
    type DomainCapability,
} from '../auth/domainAccess';

interface Domain {
    id: number;
    domain_name: string;
    php_version?: string;
    ssl_enabled?: boolean;
    status: string;
    project_type?: string;
    created_at: string;
    disk_usage?: number;
    bandwidth?: number;
    parent_id?: number | null;
    access?: DomainAccess;
    // The vhost's last observed state (D-031), sent to an administrator.
    site_config?: { state: string; adopted_from?: string; kept_by_choice?: boolean; pending_reason?: string; kind?: string };
}

// A site whose configuration file waits for the owner's choice, and the
// badge each state wears.
const SITE_CONFIG_BADGE: Record<string, TranslationKey> = {
    owner_edited: 'siteConfig.list.badge.kept',
    foreign: 'siteConfig.list.badge.kept',
    unknown_origin: 'siteConfig.list.badge.kept',
    missing: 'siteConfig.list.badge.missing',
    unreadable: 'siteConfig.list.badge.unreadable',
    // Read fine, not replaced (a locked file, set10 scenario g).
    unwritable: 'siteConfig.list.badge.unwritable',
    // A file the server did not answer for (not read): not known to be
    // CelikPanel's unchanged text; the page lists it with "Check again".
    unknown: 'siteConfig.list.badge.unknown',
};

// siteConfigBadge: the badge of a domain, or null. A kept file the owner chose
// to keep wears none, unless a certificate waits on it.
function siteConfigBadge(config: Domain['site_config']): TranslationKey | null {
    if (!config) return null;
    const key = SITE_CONFIG_BADGE[config.state];
    if (!key) return null;
    if (key === 'siteConfig.list.badge.kept' && config.kept_by_choice && !config.pending_reason) return null;
    return key;
}

const API_BASE = '/api/v1';

// A waiting deletion shown above the list. `message` is null when the saved
// marker of a pending row could not be read: the page then says exactly that.
// Listenin üstünde gösterilen bekleyen silme. Beklemedeki satırın kayıtlı
// işareti okunamadıysa `message` null olur; sayfa tam olarak bunu söyler.
interface PendingDeletion {
    id: number;
    name: string;
    message: string | null;
    // CelikPanel takes the next step by itself: the page reads again.
    automatic?: boolean;
    // CelikPanel carries on by itself: no second delete is sent for it.
    blocksRetry?: boolean;
    // The whole sentence is "nothing to do": said in one line.
    quiet?: boolean;
    // The reason, its detail and what happens next, for the guide (alpha.133).
    guide?: DeletionGuideInput;
}

// What a pending reason means for the row and the banner.
// A reason the guide words reads again on its own while CelikPanel re-checks
// it by itself (alpha.133), so a finished deletion leaves without a refresh;
// a press of Delete stays possible for it.
const pendingFlags = (reason: string, followUp?: DomainDeletionFollowUp, detail = '') => ({
    automatic: domainDeletionReasonAutomatic(reason) ||
        (domainDeletionGuideReason(reason, detail) !== null && deletionGuideFollowUp(reason, followUp, detail) === 'automatic'),
    blocksRetry: domainDeletionBlocksRetry(reason),
    quiet: domainDeletionQuiet(reason),
});

const HIDDEN_INVALID_ACCESS = 'One or more domains were hidden because their access information was invalid. / Bir veya daha fazla alan adı, erişim bilgisi geçersiz olduğu için gizlendi.';

// The rows this person may see. An administrator, reseller or customer sees
// what the server listed. A team member sees only rows whose access record is
// valid and grants something; anything else is hidden and the page says that
// rows were hidden (fail closed).
// Bu kişinin görebileceği satırlar. Ekip üyesi yalnız erişim kaydı geçerli ve
// bir şey veren satırları görür; gerisi gizlenir ve sayfa bunu söyler.
function visibleDomains(items: unknown[], teamMember: boolean): { rows: Domain[]; hiddenInvalid: boolean } {
    if (!teamMember) return { rows: items as Domain[], hiddenInvalid: false };
    let hiddenInvalid = false;
    const rows: Domain[] = [];
    for (const item of items) {
        if (!item || typeof item !== 'object' || Array.isArray(item)) {
            hiddenInvalid = true;
            continue;
        }
        const row = item as Domain & { access?: unknown };
        const access = normalizeDomainAccess(row.access);
        if (!access || !hasAnyDomainAccess(access)) {
            hiddenInvalid = true;
            continue;
        }
        rows.push({ ...row, access });
    }
    return { rows, hiddenInvalid };
}

// Plesk-style list page: breadcrumb + title, a toolbar (primary add +
// contextual remove) with search, an item count, a clean data table with
// per-row actions, and a paging footer.
//
// Plesk tarzı liste sayfası: breadcrumb + başlık, araç çubuğu (birincil
// ekle + bağlamsal kaldır) ve arama, öğe sayısı, satır-başı aksiyonlu temiz
// bir veri tablosu ve sayfalama alt bilgisi.
export function Domains() {
    const navigate = useNavigate();
    const { t, locale } = useI18n();
    const { role } = useAuth();
    const isTeamMember = role === 'additional_user';
    const [searchParams, setSearchParams] = useSearchParams();
    // Opened from the Dashboard's mail card with several domains: mailboxes
    // live on each domain's page.
    const forMail = searchParams.get('for') === 'mail';
    const [showAddModal, setShowAddModal] = useState(false);
    // The name Add domain opens with: a DNS-only zone's "Host a website".
    const [addName, setAddName] = useState('');
    // Names the new site found pointing elsewhere, as the create answered.
    const [kept, setKept] = useState<{ domain: string; outcome: SiteRecordsOutcome } | null>(null);
    const openAdd = (name = '') => { setAddName(name); setShowAddModal(true); };
    // A name filled in by a DNS-only zone's "Host a website" is a name whose DNS is served here.
    const addServedHere = addName !== '';
    const [query, setQuery] = useState('');
    const [selected, setSelected] = useState<number[]>([]);
    const [pendingDeletions, setPendingDeletions] = useState<PendingDeletion[]>([]);
    const pendingReadEpoch = useRef(0);

    // The list is in one of three states (lib/remote.ts): being read, read, or
    // not readable. "No domains yet" is a claim about the server and is drawn
    // only for an answer the server gave; a failed read says it failed and
    // offers the read again. After a failed refresh the earlier list stays,
    // marked as the earlier list, with nothing that removes a domain enabled.
    // Liste üç durumdan birindedir: okunuyor, okundu ya da okunamadı. "Henüz
    // alan adı yok" sunucu hakkında bir iddiadır ve yalnız sunucunun verdiği
    // yanıt için çizilir; başarısız okuma başarısız olduğunu söyler ve okumayı
    // yeniden sunar. Başarısız yenilemede önceki liste, önceki liste olduğu
    // belirtilerek kalır; alan adı silen hiçbir denetim etkin olmaz.
    const list = useRemote('/api/v1/domains', decodeList<unknown>);
    const listed = lastKnown(list.remote);
    const { rows: domains, hiddenInvalid } = useMemo(
        () => visibleDomains(listed?.value ?? [], isTeamMember),
        [listed?.value, isTeamMember],
    );
    const listCurrent = list.remote.state === 'known';
    // The infrastructure zone (2026-10-16): an administrator also sees the
    // zone server setup created for the panels' own names, as a DNS-only
    // entry of this list. Its own read; a failure says so in one line and
    // never hides or changes the domains.
    // Altyapi bolgesi: yonetici kurulumun olusturdugu bolgeyi bu listede
    // yalniz DNS girdisi olarak gorur. Okunamazsa tek satirda soylenir.
    const isAdmin = role === 'admin';
    const infrastructure = useRemote(isAdmin ? INFRASTRUCTURE_ZONES_URL : null, decodeInfrastructureZones);
    const infrastructureZones = lastKnown(infrastructure.remote)?.value ?? [];

    // D-009 on the page itself, not only inside the dialog: when this server is
    // KNOWN to be unable to publish a website's DNS, the Add buttons are
    // disabled with the reason and the empty state leads to where it is fixed.
    // While that is being checked, or could not be checked, the page says
    // nothing negative about DNS and Add stays available: the dialog shares
    // this same read, shows the checking line or the could-not-check notice
    // with Retry, and keeps its own submit disabled until the answer is known.
    // The backend refuses an unpublishable domain in every case.
    // D-009 yalnız pencerede değil sayfanın kendisinde: bu sunucunun bir web
    // sitesinin DNS'ini yayımlayamadığı BİLİNİYORSA Ekle düğmeleri gerekçesiyle
    // pasiftir ve boş durum düzeltileceği yere götürür. Bu kontrol edilirken ya
    // da edilemediğinde sayfa DNS hakkında olumsuz bir şey söylemez ve Ekle
    // kullanılabilir kalır: pencere aynı okumayı paylaşır, kontrol satırını ya
    // da Tekrar dene'li bildirimi gösterir ve yanıt bilinene kadar kendi
    // gönder düğmesini kapalı tutar.
    const capabilities = useHostingCapabilities({ enabled: !isTeamMember });
    const dns = gateOn(capabilities.remote, (value) => dnsBlocker(value, 'website'));
    const dnsBlocked = !isTeamMember && dns.state === 'blocked' ? dns.reason : null;
    // The Dashboard's "Host a website" card arrives as /domains?host=<name>: the
    // same dialog as the row button opens once, with the name filled in. The
    // address is cleaned so a reload does not open it again.
    // Panonun "Web sitesi barindir" karti /domains?host=<ad> ile gelir.
    const hostParam = searchParams.get('host');
    const hostHandled = useRef(false);
    useEffect(() => {
        if (!hostParam || hostHandled.current || isTeamMember) return;
        hostHandled.current = true;
        openAdd(hostParam);
        const next = new URLSearchParams(searchParams);
        next.delete('host');
        setSearchParams(next, { replace: true });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [hostParam, isTeamMember]);
    // Whether an engine is missing or only its identity is, the DNS
    // infrastructure section is where it gets fixed; the Services page can no
    // longer install a DNS engine (DNS_ENGINE_WORKFLOW_REQUIRED), so a fresh
    // host sent there had nowhere to go (R-029, screen side).
    // Motor mu eksik yoksa yalnız kimliği mi, düzelten yer DNS altyapısı
    // bölümüdür; Servisler sayfası artık DNS motoru kuramaz
    // (DNS_ENGINE_WORKFLOW_REQUIRED), oraya gönderilen taze sunucunun gidecek
    // yeri yoktu (R-029, ekran tarafı).
    const openDNSRequirement = () => navigate('/dns-infrastructure');
    const [deleteRefusal, setDeleteRefusal] = useState<ApiError | null>(null);
    // Name the half that is missing: no engine at all, or an engine whose
    // identity is not staged yet. The same key feeds the button label.
    // Eksik yarıyı adlandır: hiç motor yok ya da kimliği henüz hazırlanmamış
    // bir motor var. Aynı anahtar düğme etiketini de besler.
    const dnsRequirementText = dnsBlocked === null
        ? undefined
        : dnsBlocked === 'engine'
            // Why the engine half is missing, by its state: a main DNS server
            // waiting for its backup is not "no DNS" (owner report 2026-10-31).
            ? capabilities.remote.state === 'known' ? dnsEngineBlockerText(capabilities.remote.value, t) : t('domains.add.needsDns')
            : t('err.DNS_SETTINGS_REQUIRED');

    const pendingMessage = (reason: string, detail = '', next?: Pick<DeletionGuideInput, 'followUp' | 'nextCheckAt'>, place: DeletionGuidePlace = 'banner') => {
        // A reason the deletion guide words (alpha.133): why, who acts, the
        // next step and how it continues, the same sentences as the banner;
        // the window and the bulk notes point to the banner's Details.
        const words = deletionGuideWords({ reason, detail, ...next }, isAdmin, t, locale, place);
        if (words) return deletionGuideText(words, t);
        const key = domainDeletionReasonKey(reason) as TranslationKey | null;
        const translated = key ? t(key) : '';
        if (!key || translated === key) return t('domains.deletionPending');
        // What the secondary's inspector reported, or which check of this
        // server's proof differed, as its own sentence after the reason's
        // guidance. Only reviewed tokens reach this point.
        const detailKey = domainDeletionDetailKey(reason, detail) as TranslationKey | null;
        const detailText = detailKey ? t(detailKey) : '';
        return detailKey && detailText !== detailKey ? `${translated} ${detailText}` : translated;
    };

    // Every row the server lists as pending is asked for its saved deletion
    // marker, after each answer of the list. These are reads. A marker that
    // could not be read is shown as exactly that; it is not "no deletion is
    // waiting".
    // Sunucunun beklemede listelediği her satır için kayıtlı silme işareti,
    // listenin her yanıtından sonra sorulur. Bunlar okumadır. Okunamayan
    // işaret tam olarak öyle gösterilir; "bekleyen silme yok" sayılmaz.
    const listedAt = listed?.observedAt;
    useEffect(() => {
        const epoch = ++pendingReadEpoch.current;
        // Without a current answer of the list there is nothing to ask about;
        // what an earlier answer established stays on screen with that list.
        // Listenin güncel yanıtı yokken sorulacak bir şey yoktur; önceki
        // yanıtın saptadığı, o listeyle birlikte ekranda kalır.
        if (isTeamMember || !listCurrent) return;
        const pending = domains.filter((domain) => domain.status === 'pending');
        let finished = false;
        void Promise.all(pending.map(async (domain): Promise<PendingDeletion | null> => {
            const saved = await readSavedDomainDeletion(domain.id);
            if (saved.state === 'gone') {
                // The deletion finished after the list answered: read again so
                // the row leaves without a refresh (lab run 6).
                finished = true;
                return null;
            }
            if (saved.state === 'unknown') return { id: domain.id, name: domain.domain_name, message: null };
            return saved.saved === null ? null : {
                id: domain.id, name: domain.domain_name, message: pendingMessage(saved.saved.reason, saved.saved.detail, saved.saved),
                ...pendingFlags(saved.saved.reason, saved.saved.followUp, saved.saved.detail),
                guide: { reason: saved.saved.reason, detail: saved.saved.detail, followUp: saved.saved.followUp, nextCheckAt: saved.saved.nextCheckAt },
            };
        })).then((observed) => {
            if (epoch === pendingReadEpoch.current) {
                setPendingDeletions(observed.filter((item): item is PendingDeletion => item !== null));
                if (finished) invalidate('domains', 'dns');
            }
        });
        // One run per answer of the list: `listedAt` changes with every read.
    }, [listedAt, listCurrent, isTeamMember]);

    const reload = () => void list.retry();
    // After a change, every place that shows domains reads again: this list,
    // the infrastructure rows, the navigation counter, the dashboard cards and
    // the plan usage (no page refresh, owner rule 2026-10-31). It only reads.
    // Degisiklikten sonra alan adlarini gosteren her yer yeniden okunur.
    const changed = () => { invalidate('domains', 'dns'); };

    // While a shown deletion is one CelikPanel continues by itself (for
    // example a joined server waiting for the main DNS server), every place
    // that shows domains reads again after 15 s, so a finished deletion leaves
    // the list without a refresh (owner rule 2026-10-31). It only reads.
    // Kendiliğinden süren bir silme varken alan adları 15 sn sonra yeniden okunur.
    // Any other waiting deletion is read again every minute (alpha.133 round
    // 5, lab w9: a second open tab kept a deletion that waited for a person
    // after it was finished from another tab, because only "automatic" ones
    // were read again). These are the same reads; they start nothing.
    // Bekleyen her silme dakikada bir yeniden okunur; yalnız okumadır.
    const autoRecheck = pendingDeletions.some((item) => item.automatic);
    const watchPending = pendingDeletions.length > 0;
    useEffect(() => {
        if (!watchPending) return;
        const timer = window.setTimeout(() => invalidate('domains', 'dns'), autoRecheck ? 15000 : 60000);
        return () => window.clearTimeout(timer);
    }, [watchPending, autoRecheck, listedAt]);

    // A domain is "being deleted" while its delete request is on its way, and
    // while the server says CelikPanel carries the deletion on by itself. Its
    // row then offers no delete, no settings and no selection, and a re-press
    // can never send a second delete (owner report 6 Nov 2026).
    // Silme istegi yoldayken ve sunucu silmeyi CelikPanel kendisi surdururken
    // alan adi "siliniyor"dur; ikinci silme gonderilmez.
    const [deleting, setDeleting] = useState<number[]>([]);
    const inFlight = useRef(new Set<number>());
    const beingDeleted = (id: number) =>
        deleting.includes(id) || pendingDeletions.some((item) => item.id === id && item.blocksRetry);

    // A row's two visible actions, as words (P3).
    // Satırın iki görünür işlemi, sözcükle.
    const manageAction = (d: Domain): RowAction => ({
        id: 'manage',
        label: t('domains.action.manage'),
        icon: Settings,
        onSelect: () => navigate(`/domains/${encodeURIComponent(d.domain_name)}`),
        disabled: beingDeleted(d.id),
        disabledReason: t('domains.status.beingDeleted'),
    });
    const visitAction = (d: Domain): RowAction => ({
        id: 'visit',
        label: t('domains.action.visit'),
        icon: ExternalLink,
        href: `https://${d.domain_name}`,
    });
    const zoneManage = (zone: string): RowAction => ({
        id: 'manage',
        label: t('domains.infrastructure.manage'),
        icon: Settings,
        onSelect: () => navigate(infrastructureZonePath(zone)),
    });

    // The question names the domain and what goes with it. Once confirmed the
    // same window shows the deletion's steps, locked, until it ends: this
    // server's part, the DNS servers' part with what it waits for, and the
    // finish (owner's firm order, 6 Nov 2026). It only reads the deletion's
    // saved state while it waits; it never sends a second delete.
    // Soru alan adini ve onunla gideni soyler; onaydan sonra ayni pencere
    // silmenin adimlarini kilitli gosterir. Beklerken yalniz okur.
    // What goes and what stays is what the server's deletion does
    // (cmd/panel/domain_handlers.go handleDeleteDomain, alpha.133): the site
    // with its files and FTP logins, mail, CelikPanel's DNS records,
    // certificates and backup schedules go; linked databases stay on their
    // engine and backups stay where they are, both out of the panel's sight;
    // stored mail is moved aside. Kinds only: the list knows no counts. The
    // name is typed before Delete turns on (owner, alpha.132 proposal).
    // Silmenin götürdüğü ve bıraktığı, sunucunun yaptığıdır; ad yazılır.
    const deleteLists = () => ({
        lost: [t('domains.delete.lost.site'), t('domains.delete.lost.mail'), t('domains.delete.lost.dns'), t('domains.delete.lost.schedule')],
        kept: [t('domains.delete.kept.databases'), t('domains.delete.kept.backups'), t('domains.delete.kept.mail'), t('domains.delete.kept.dns')],
    });
    const handleDelete = async (id: number, name: string) => {
        if (!listCurrent || beingDeleted(id)) return;
        await confirmAction({
            title: t('domains.deleteTitle', { name }),
            message: t('domains.deleteWhat'),
            ...deleteLists(),
            typedName: { value: name, label: t('domains.delete.typeName', { name }), ignoreCase: true },
            confirmLabel: t('common.delete'),
            tone: 'danger',
            progressTitle: t('domains.deletingTitle', { name }),
            action: (report) => deleteAndFollow(id, name, report),
        });
    };

    // One confirmation for a bulk delete, naming the count and the domains;
    // then each is deleted in turn, one step each, each with its own outcome.
    // Toplu silmede tek onay: sayi ve adlar; sonra her biri sirayla, birer adim.
    const handleBulkDelete = async (picked: Domain[]) => {
        const rows = picked.filter((row) => !beingDeleted(row.id));
        if (!listCurrent || rows.length === 0) return;
        if (rows.length === 1) {
            await handleDelete(rows[0].id, rows[0].domain_name);
            return;
        }
        await confirmAction({
            title: t('domains.bulkDeleteTitle', { n: rows.length }),
            message: t('domains.confirmBulkDelete', { n: rows.length, names: rows.map((d) => d.domain_name).join('\n') }),
            ...deleteLists(),
            confirmLabel: t('common.delete'),
            tone: 'danger',
            progressTitle: t('domains.bulkDeletingTitle', { n: rows.length }),
            action: async (report) => {
                const steps: OperationStep[] = rows.map((row) => ({ id: String(row.id), label: row.domain_name, state: 'pending' }));
                const show = (index: number, step: Partial<OperationStep>) => {
                    steps[index] = { ...steps[index], ...step };
                    report.steps([...steps]);
                };
                let unfinished = 0;
                for (const [index, row] of rows.entries()) {
                    show(index, { state: 'running', since: Date.now() });
                    const result = await deleteDomain(row.id, row.domain_name);
                    if (result.kind === 'done') {
                        show(index, { state: 'done', note: result.note });
                    } else {
                        unfinished++;
                        show(index, result.kind === 'pending' && continuesByItself(result.reason, result.followUp)
                            ? { state: 'waiting', note: t('domains.step.continuesAlone') }
                            : { state: 'failed', note: deleteResultText(result) });
                    }
                }
                return unfinished === 0 ? { state: 'done' } : { state: 'blocked', reason: t('domains.bulkUnfinished', { n: unfinished }) };
            },
        });
    };

    // A deletion still running after a reload, or one this page just learnt
    // of, is followed in the same locked window: the page is not free while
    // CelikPanel carries it on by itself.
    // Yeniden yuklemeden sonra hala suren silme ayni kilitli pencerede izlenir.
    const followed = useRef(new Set<number>());
    // A page that is gone stops reading (its window ends with it).
    const alive = useRef(true);
    useEffect(() => () => { alive.current = false; }, []);
    useEffect(() => {
        const active = pendingDeletions.find((item) => item.blocksRetry && !followed.current.has(item.id) && !inFlight.current.has(item.id));
        if (!active) return;
        followed.current.add(active.id);
        void followOperation({
            progressTitle: t('domains.deletingTitle', { name: active.name }),
            tone: 'danger',
            action: (report) => followDeletion(active.id, active.name, report, Date.now()),
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [pendingDeletions]);

    type DeleteResult =
        | { kind: 'done'; note?: string }
        | ({ kind: 'pending'; reason: string; detail?: string; stage: string } & Pick<DeletionGuideInput, 'followUp' | 'nextCheckAt'>)
        | { kind: 'failed'; text: string }
        | { kind: 'unknown' };

    const deleteResultText = (result: DeleteResult) =>
        result.kind === 'failed' ? result.text
            : result.kind === 'pending' ? pendingMessage(result.reason, result.detail, result, 'window')
                : result.kind === 'unknown' ? t('common.resultUnknown') : '';

    const deleteDomain = async (id: number, name: string): Promise<DeleteResult> => {
        // Never two delete requests for one domain at a time.
        if (inFlight.current.has(id)) return { kind: 'pending', reason: 'deletion_running', stage: 'running' };
        inFlight.current.add(id);
        followed.current.add(id);
        setDeleting((current) => [...current, id]);
        try {
            return await sendDelete(id, name);
        } finally {
            inFlight.current.delete(id);
            setDeleting((current) => current.filter((item) => item !== id));
        }
    };

    // The steps of a deletion, in the server's order. While this server's
    // request runs the server names no sub-step, so one step says what it is
    // doing; once the server names the stage the deletion is at (or stopped
    // at), each step is shown with its own state (Go: DELETE and
    // deletion-status answer with `stage`).
    // Silmenin adimlari sunucunun sirasiyla; sunucu asamayi bildirince her
    // adim kendi durumuyla gosterilir.
    const STEP_ORDER = ['mail', 'site', 'dns', 'certificates', 'finish'] as const;
    const STAGE_STEP: Record<string, (typeof STEP_ORDER)[number]> = {
        mail_tls_cleanup: 'mail', mail_runtime_cleanup: 'mail', site_cleanup: 'site', dns_cleanup: 'dns',
        certificate_cleanup: 'certificates', ledger_delete: 'finish', dns_removed: 'finish',
    };
    const requestSteps = (name: string, since: number): OperationStep[] => [
        { id: 'request', label: t('domains.step.request', { name }), state: 'running', since },
        { id: 'finish', label: t('domains.step.finish'), state: 'pending' },
    ];
    const deletionSteps = (name: string, stage: string, reason: string, since: number, failed = false, note?: string): OperationStep[] => {
        const waitNote: Record<string, string> = {
            remote_dns_removal_confirming: t('domains.step.note.confirming'),
            dns_peer_catching_up: t('domains.step.note.catchingUp'),
            remote_dns_main_unreachable: t('domains.step.note.mainUnreachable'),
            // alpha.133: removed here; the backup's confirmation is re-checked
            // by CelikPanel itself. Waiting, not failed.
            dns_peer_proof_timeout: t('domains.step.note.verifying'),
            dns_peer_journal_unknown: t('domains.step.note.verifying'),
        };
        // The joined server's DNS removal is done once the main server confirmed it.
        const step = reason === 'remote_dns_removed_finishing' ? 'finish' : STAGE_STEP[stage];
        if (!step) return requestSteps(name, since);
        const at = STEP_ORDER.indexOf(step);
        const labels = {
            mail: t('domains.step.mail'),
            site: t('domains.step.site'),
            dns: t('domains.step.dns', { name }),
            certificates: t('domains.step.certificates'),
            finish: t('domains.step.finish'),
        };
        // A finished step says what is done, not what is being done.
        const doneLabels = {
            mail: t('domains.step.done.mail'),
            site: t('domains.step.done.site'),
            dns: t('domains.step.done.dns', { name }),
            certificates: t('domains.step.done.certificates'),
            finish: t('domains.step.done.finish'),
        };
        const waiting = note ?? waitNote[reason];
        const current: OperationStep['state'] = failed ? 'failed' : waiting ? 'waiting' : 'running';
        return STEP_ORDER.map((id, index) => ({
            id,
            label: index < at ? doneLabels[id] : labels[id],
            state: index < at ? 'done' : index > at ? 'pending' : current,
            ...(index === at ? { since, note: failed ? undefined : waiting } : {}),
        }));
    };

    // How long the window stays locked on a deletion that waits for another
    // server; past it, the window says the deletion continues by itself and
    // can be closed (the row keeps saying "Being deleted").
    const FOLLOW_LIMIT_MS = 10 * 60_000;

    // Reasons of this server's DNS step that the Panel re-checks by itself,
    // while a press of "Retry this deletion" stays possible.
    const RECHECKED_BY_ITSELF = new Set(['dns_peer_proof_timeout', 'dns_peer_journal_unknown']);
    // While the server says CelikPanel re-checks it by itself (follow_up
    // automatic): only then do the window and the bulk notes say that nobody
    // needs to act and that it continues alone (alpha.133 review).
    const continuesByItself = (reason: string, followUp?: DeletionGuideInput['followUp']) =>
        (RECHECKED_BY_ITSELF.has(reason) || domainDeletionBlocksRetry(reason)) &&
        (domainDeletionGuideReason(reason) === null || deletionGuideFollowUp(reason, followUp) === 'automatic');

    // How a pending answer ends the window when CelikPanel does not carry it
    // on by itself: the step it stopped at is marked, and the reason (who
    // acts, the next step) is said.
    const pendingEnd = (name: string, report: OperationReport, stage: string, reason: string, detail: string | undefined, since: number,
        next?: Pick<DeletionGuideInput, 'followUp' | 'nextCheckAt'>): OperationEnd => {
        if (continuesByItself(reason, next?.followUp)) {
            // Nothing failed: the removal is done here and CelikPanel checks
            // the backup's confirmation again by itself (alpha.133).
            report.steps(deletionSteps(name, stage, reason, since));
            // "Someone needs to act" would not be true here: nobody does.
            return { state: 'blocked', subtitle: t('domains.step.rechecksAlone'), reason: `${pendingMessage(reason, detail, next, 'window')} ${t('domains.step.continuesAlone')}` };
        }
        if (domainDeletionGuideReason(reason) !== null) {
            // Not a failure of the deletion: it waits for the server's
            // administrator (stuck, checks ended, or a check record CelikPanel
            // cannot sort out), who is named with the one next step.
            report.steps(deletionSteps(name, stage, reason, since, false, t('domains.deletionGuide.stepNote.person')));
            return { state: 'blocked', reason: pendingMessage(reason, detail, next, 'window') };
        }
        report.steps(deletionSteps(name, stage, reason, since, true));
        return { state: reason ? 'failed' : 'unknown', reason: pendingMessage(reason, detail) };
    };

    // Reads the deletion's saved state every 3 s until it ends. Reads only.
    const followDeletion = async (id: number, name: string, report: OperationReport, startedAt: number): Promise<OperationEnd> => {
        let last = '';
        let stepSince = Date.now();
        let unknownSince: number | null = null;
        for (;;) {
            if (!alive.current) return { state: 'blocked', reason: t('domains.step.continuesAlone') };
            const saved = await readSavedDomainDeletionWithStage(id);
            if (saved.state === 'gone') {
                pendingReadEpoch.current++;
                setPendingDeletions((current) => current.filter((item) => item.id !== id));
                report.steps(deletionSteps(name, 'ledger_delete', '', 0).map((step) => ({ ...step, state: 'done' })));
                changed();
                return { state: 'done', message: t('domains.deleted', { name }) };
            }
            if (saved.state === 'unknown') {
                unknownSince ??= Date.now();
                if (Date.now() - unknownSince > 60_000) {
                    changed();
                    return { state: 'unknown', reason: t('domains.step.unread', { name }) };
                }
            } else {
                unknownSince = null;
                if (saved.saved === null) {
                    // No deletion is recorded and the domain is still here.
                    changed();
                    return { state: 'unknown', reason: t('domains.step.stopped', { name }) };
                }
                const { reason, detail } = saved.saved;
                const next = { followUp: saved.saved.followUp, nextCheckAt: saved.saved.nextCheckAt };
                if (`${saved.stage}/${reason}` !== last) {
                    last = `${saved.stage}/${reason}`;
                    stepSince = Date.now();
                }
                // A reason the guide words whose follow-up is not CelikPanel's
                // own re-check ends the window with who acts (alpha.133).
                if (!domainDeletionBlocksRetry(reason) ||
                    (domainDeletionGuideReason(reason) !== null && !continuesByItself(reason, next.followUp))) {
                    changed();
                    return pendingEnd(name, report, saved.stage, reason, detail, stepSince, next);
                }
                report.steps(deletionSteps(name, saved.stage, reason, stepSince));
                if (Date.now() - startedAt > FOLLOW_LIMIT_MS) {
                    changed();
                    return { state: 'blocked', reason: `${pendingMessage(reason, detail, next, 'window')} ${t('domains.step.continuesAlone')}` };
                }
            }
            await new Promise((resolve) => window.setTimeout(resolve, 3000));
        }
    };

    // While the delete request itself runs, the saved state is read every
    // 1.5 s so the window shows the step the server is at now (Go: the
    // running answer names `step`). Reads only; it never sends a change and
    // stops as soon as the request answers.
    // Silme istegi surerken sunucunun su anki adimi okunur; yalniz okur.
    const watchRunningDeletion = (id: number, name: string, report: OperationReport, isDone: () => boolean) => {
        void (async () => {
            let last = '';
            let since = Date.now();
            while (alive.current && !isDone()) {
                await new Promise((resolve) => setTimeout(resolve, 1500));
                if (!alive.current || isDone()) return;
                const saved = await readSavedDomainDeletionWithStage(id);
                if (isDone()) return;
                if (saved.state !== 'known' || saved.saved?.reason !== 'deletion_running' || !STAGE_STEP[saved.stage]) continue;
                if (saved.stage !== last) {
                    last = saved.stage;
                    since = Date.now();
                }
                report.steps(deletionSteps(name, saved.stage, 'deletion_running', since));
            }
        })();
    };

    const deleteAndFollow = async (id: number, name: string, report: OperationReport): Promise<OperationEnd> => {
        const startedAt = Date.now();
        report.steps(requestSteps(name, startedAt));
        let answered = false;
        watchRunningDeletion(id, name, report, () => answered);
        const result = await deleteDomain(id, name).finally(() => { answered = true; });
        if (result.kind === 'done') {
            report.steps(deletionSteps(name, 'ledger_delete', '', startedAt).map((step) => ({ ...step, state: 'done' })));
            return { state: 'done', message: result.note ?? t('domains.deleted', { name }) };
        }
        if (result.kind === 'failed') return { state: 'failed', reason: result.text };
        if (result.kind === 'unknown') return { state: 'unknown', reason: t('common.resultUnknown') };
        if (!domainDeletionBlocksRetry(result.reason) ||
            (domainDeletionGuideReason(result.reason) !== null && !continuesByItself(result.reason, result.followUp))) {
            return pendingEnd(name, report, result.stage, result.reason, result.detail, Date.now(), result);
        }
        report.steps(deletionSteps(name, result.stage, result.reason, Date.now()));
        return followDeletion(id, name, report, startedAt);
    };

    const sendDelete = async (id: number, name: string): Promise<DeleteResult> => {
        try {
            const res = await fetch(`${API_BASE}/domains/${id}`, { method: 'DELETE' });
            const stage = await readDomainDeletionStage(res);
            const outcome = await readDomainDeletionOutcome(res);
            if (outcome.state === 'pending') {
                const message = pendingMessage(outcome.reason, outcome.detail, outcome);
                pendingReadEpoch.current++;
                setPendingDeletions((current) => [
                    ...current.filter((item) => item.id !== id),
                    {
                        id, name, message, ...pendingFlags(outcome.reason, outcome.followUp),
                        guide: { reason: outcome.reason, detail: outcome.detail, followUp: outcome.followUp, nextCheckAt: outcome.nextCheckAt },
                    },
                ]);
                // No toast: the window and the banner above the list say it.
                setSelected((s) => s.filter((x) => x !== id));
                changed();
                return { kind: 'pending', reason: outcome.reason, detail: outcome.detail, stage, followUp: outcome.followUp, nextCheckAt: outcome.nextCheckAt };
            }
            if (outcome.state === 'error') {
                // The refusal and its fix stay at the top of the list until the
                // next delete (owner feedback 2026-10-31); it is said there, and
                // in the window that asked, never again in a notice (useRefusal).
                const apiError = await readApiError(res);
                setDeleteRefusal(apiError);
                return { kind: 'failed', text: refusalText(apiError, t, 'domains.deleteFailed') };
            }
            setDeleteRefusal(null);
            pendingReadEpoch.current++;
            setPendingDeletions((current) => current.filter((item) => item.id !== id));
            // A DKIM key or table line CelikPanel did not make was kept, or
            // the key could not be removed: said, with what the owner does.
            const dkim = await readDomainDeletionDKIM(res);
            const note = dkim?.failed
                ? t('domains.deletedDKIMFailed', { name })
                : dkim?.keptKey
                    ? t('domains.deletedDKIMKept', { name, path: dkim.keptKey })
                    : dkim?.ownerLines ? t('domains.deletedDKIMLines', { name }) : undefined;
            setSelected((s) => s.filter((x) => x !== id));
            changed();
            return { kind: 'done', note };
        } catch {
            // No answer: it is not known whether the domain was deleted; the list is read again.
            changed();
            return { kind: 'unknown' };
        }
    };

    const filtered = domains.filter((d) => d.domain_name.toLowerCase().includes(query.toLowerCase()));
    // A zone a website now uses is listed as that website, with a badge.
    const infrastructureRows = infrastructureZones.filter((zone) => zone.domainId === null && zone.zone.includes(query.toLowerCase()));
    const infrastructureNames = new Set(infrastructureZones.map((zone) => zone.zone));
    const listedCount = filtered.length + infrastructureRows.length;
    const allSelected = !isTeamMember && filtered.length > 0 && selected.length === filtered.length;
    const canView = (domain: Domain, capability: DomainCapability) =>
        !isTeamMember || Boolean(domain.access && hasDomainAccess(domain.access, capability, 'view'));
    const addButton = (
        <span title={dnsRequirementText}>
            <Button variant="primary" icon={Plus} disabled={dnsBlocked !== null} aria-describedby={dnsBlocked !== null && domains.length > 0 ? 'domains-add-blocked' : undefined} onClick={() => openAdd()}>
                {t('domains.add')}
            </Button>
        </span>
    );

    return (
        <div className="p-6 md:p-8">
            {/* The title first, then the plan cards, on every width (on a
                phone the cards stood above the title). */}
            <PageHeader
                title={t('nav.domains')}
                subtitle={hiddenInvalid ? HIDDEN_INVALID_ACCESS : t('domains.subtitle')}
                breadcrumb={[t('common.home'), t('nav.domains')]}
                actions={!isTeamMember && addButton}
            />
            {!isTeamMember && <SubscriptionUsage />}

            {/* Why Add domain is off, at the top and with its fix; the empty
                list says it in its own place (owner feedback 2026-10-31). */}
            {dnsBlocked !== null && domains.length > 0 && (
                <BlockingProblems id="domains-add-blocked" className="mb-4" title={t('domains.add.blockedTitle')} problems={[{
                    code: dnsBlocked === 'engine' ? 'DNS_SERVER_REQUIRED' : 'DNS_SETTINGS_REQUIRED',
                    what: dnsRequirementText ?? '',
                    fix: { label: t(dnsBlocked === 'engine' ? 'err.DNS_SERVER_REQUIRED.action' : 'err.DNS_SETTINGS_REQUIRED.action'), onFix: openDNSRequirement },
                }]} />
            )}

            <ErrorBanner className="mb-4" error={deleteRefusal} sentence="domains.deleteFailed" />

            {forMail && (
                <p role="status" className="mb-4 rounded-lg border border-border bg-surface-2 p-3 text-sm text-fg" data-domains-for-mail="">
                    {t('domains.forMail')}
                </p>
            )}

            {kept && <KeptSiteRecords domain={kept.domain} outcome={kept.outcome} onDismiss={() => setKept(null)} />}

            {pendingDeletions.length > 0 && <DNSCheckToolNotice className="mb-4" />}
            {pendingDeletions.map((pending) => (pending.message === null ? (
                <CouldNotCheck
                    key={pending.id}
                    className="mb-4"
                    text={t('domains.pendingUnknown', { name: pending.name })}
                    onRetry={reload}
                    busy={list.reading}
                />
            ) : (
                pending.quiet ? (
                <p key={pending.id} role="status" className="mb-4 flex items-center gap-2 rounded-lg border border-border-strong bg-surface-2 p-3 text-sm text-fg" data-domain-being-deleted={pending.name}>
                    <Spinner size="xs" />
                    {t('domains.beingDeletedLine', { name: pending.name })}
                </p>
            ) : (
                pending.guide && deletionGuideWords(pending.guide, isAdmin, t, locale) ? (
                    <DeletionWaitingGuide key={pending.id} name={pending.name} input={pending.guide} admin={isAdmin} onCheck={reload} />
                ) : (
                    <Guidance
                        key={pending.id}
                        className="mb-4"
                        tone="blocked"
                        title={t('domains.deletionWaiting', { name: pending.name })}
                        why={pending.message}
                        action={<Button variant="secondary" onClick={reload}>{t('domains.checkDeletionStatus')}</Button>}
                    />
                )
            ))))}

            <RemoteGate
                remote={list.remote}
                checking={t('domains.checking')}
                failed={t('domains.unknown')}
                onRetry={reload}
                busy={list.reading}
                className="py-1"
            >
                {(shown) => (domains.length === 0 && infrastructureZones.every((zone) => zone.domainId !== null) ? (
                <KnownEmpty
                    of={shown}
                    icon={Globe}
                    title={t('domains.empty')}
                    hint={dnsRequirementText ?? t('domains.emptyHint')}
                    action={!isTeamMember && (
                        dnsBlocked !== null ? (
                            // The honest next step is not a dead Add button but
                            // the page where the requirement is met.
                            // Dürüst sonraki adım ölü bir Ekle düğmesi değil,
                            // gereksinimin karşılandığı sayfadır.
                            <Button variant="primary" icon={Settings} onClick={openDNSRequirement}>
                                {dnsBlocked === 'engine'
                                    ? t('err.DNS_SERVER_REQUIRED.action')
                                    : t('err.DNS_SETTINGS_REQUIRED.action')}
                            </Button>
                        ) : (
                            <Button variant="primary" icon={Plus} onClick={() => openAdd()}>
                                {t('domains.add')}
                            </Button>
                        )
                    )}
                />
            ) : (
                <div className="rounded-xl border border-border-strong bg-surface">
                    {isAdmin && infrastructure.remote.state === 'unknown' && (
                        <CouldNotCheck
                            className="m-3"
                            text={t('domains.infrastructure.unknown')}
                            onRetry={() => void infrastructure.retry()}
                            busy={infrastructure.reading}
                        />
                    )}
                    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-3">
                        {!isTeamMember && <div className="flex items-center gap-2">
                            {addButton}
                            {selected.length > 0 && (
                                <Button
                                    variant="danger"
                                    icon={Trash2}
                                    disabled={shown.stale || filtered.every((d) => !selected.includes(d.id) || beingDeleted(d.id))}
                                    onClick={() => void handleBulkDelete(filtered.filter((d) => selected.includes(d.id)))}
                                >
                                    {t('common.remove')} ({selected.length})
                                </Button>
                            )}
                        </div>}
                        <SearchInput value={query} onChange={setQuery} placeholder={t('domains.search')} />
                    </div>

                    <p className="px-4 pt-3 text-xs text-fg-subtle">
                        {(listedCount === 1 ? t('common.itemsTotalOne') : t('common.itemsTotal', { n: listedCount }))}
                    </p>
                    {(() => {
                        // The first state after an update from a release that
                        // wrote no header (D-031): how many files were taken
                        // over and how many were left alone. Computed from the
                        // owner's decisions, not stored: the line stays while a
                        // left-alone file has no decision for its current bytes
                        // ("keep mine"; "take" makes it CelikPanel's text) and
                        // is gone once each has one. When nothing was left alone
                        // nothing waits for the owner and the line is not drawn.
                        // Başlıksız sürümden güncellemeden sonraki ilk durum;
                        // olduğu gibi bırakılan her dosya için karar verilince
                        // satır kalkar.
                        const adopted = domains.filter((d) => d.site_config?.adopted_from).length;
                        const left = domains.filter((d) => d.site_config?.state === 'unknown_origin').length;
                        const undecided = domains.filter((d) => d.site_config?.state === 'unknown_origin' && !d.site_config.kept_by_choice).length;
                        return undecided > 0 ? (
                            <p role="status" className="mx-4 mt-2 rounded-lg border border-border-strong bg-surface-2 p-3 text-sm text-fg" data-site-config-first-state>
                                {t('siteConfig.list.firstState', { adopted, left })}
                            </p>
                        ) : null;
                    })()}

                    {/* On a phone every row is a card: its facts one under the
                        other and its actions, as words, under them (P3). */}
                    <div className="overflow-x-auto">
                        <table className="w-full text-sm max-sm:block">
                            <thead className={isTeamMember ? 'max-sm:hidden' : 'max-sm:block'}>
                                <tr className="border-b border-border text-left text-xs font-semibold text-fg-muted max-sm:flex">
                                    {!isTeamMember && <th className="w-10 px-4 py-2.5">
                                        <input
                                            type="checkbox"
                                            checked={allSelected}
                                            disabled={shown.stale}
                                            onChange={() =>
                                                setSelected(allSelected ? [] : filtered.filter((d) => !beingDeleted(d.id)).map((d) => d.id))
                                            }
                                            className="h-4 w-4 accent-primary"
                                        />
                                    </th>}
                                    <th className="px-4 py-2.5 max-sm:hidden">{t('domains.col.name')}</th>
                                    <th className="px-4 py-2.5 max-sm:hidden">{t('domains.col.php')}</th>
                                    <th className="px-4 py-2.5 text-right max-sm:hidden">{t('domains.col.disk')}</th>
                                    <th className="px-4 py-2.5 text-right max-sm:hidden">{t('domains.col.traffic')}</th>
                                    <th className="px-4 py-2.5 max-sm:hidden">{t('domains.col.status')}</th>
                                    <th className="px-4 py-2.5 max-sm:hidden" />
                                </tr>
                            </thead>
                            <tbody className="max-sm:block">
                                {filtered.map((d) => (
                                    <tr key={d.id} className="border-b border-border last:border-0 hover:bg-surface-2/60 max-sm:flex max-sm:flex-wrap max-sm:items-start max-sm:gap-x-3 max-sm:gap-y-1.5 max-sm:p-4">
                                        {!isTeamMember && <td className="px-4 py-3 max-sm:p-0 max-sm:pt-0.5">
                                            <input
                                                type="checkbox"
                                                checked={selected.includes(d.id)}
                                                disabled={shown.stale || beingDeleted(d.id)}
                                                onChange={() =>
                                                    setSelected((s) =>
                                                        s.includes(d.id) ? s.filter((x) => x !== d.id) : [...s, d.id],
                                                    )
                                                }
                                                className="h-4 w-4 accent-primary"
                                            />
                                        </td>}
                                        <td className="px-4 py-3 max-sm:min-w-0 max-sm:flex-1 max-sm:p-0">
                                            {/* The badges drop below the name on a narrow screen;
                                                the icon stays with the name. */}
                                            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                                                <span className="inline-flex min-w-0 items-center gap-2">
                                                    {canView(d, 'ssl') && d.ssl_enabled ? (
                                                        <Lock className="h-4 w-4 shrink-0 text-success" />
                                                    ) : (
                                                        <Globe className="h-4 w-4 shrink-0 text-fg-subtle" />
                                                    )}
                                                    <button
                                                        onClick={() =>
                                                            navigate(`/domains/${encodeURIComponent(d.domain_name)}`)
                                                        }
                                                        className="text-left text-base font-medium text-primary hover:underline"
                                                    >
                                                        {d.domain_name}
                                                    </button>
                                                </span>
                                                {infrastructureNames.has(d.domain_name) && (
                                                    <span className="rounded-md bg-surface-2 px-1.5 py-0.5 text-xs font-medium text-fg-muted" data-infrastructure-site="">
                                                        {t('domains.infrastructure.siteBadge')}
                                                    </span>
                                                )}
                                                {d.parent_id ? (
                                                    <span className="rounded-md bg-surface-2 px-1.5 py-0.5 text-xs font-medium text-fg-subtle">
                                                        {t('domains.subdomain')}
                                                    </span>
                                                ) : null}
                                                {(() => {
                                                    const badge = siteConfigBadge(d.site_config);
                                                    return badge ? (
                                                        <span className="rounded-md border border-warning-mark/50 bg-warning-mark/20 px-1.5 py-0.5 text-xs font-medium text-fg" data-site-config-badge={d.site_config?.state} data-site-config-badge-file={d.site_config?.kind || 'nginx_vhost'}>
                                                            {t(badge)}
                                                        </span>
                                                    ) : null;
                                                })()}
                                            </div>
                                        </td>
                                        <td className="px-4 py-3 max-sm:basis-full max-sm:p-0">
                                            {canView(d, 'php') ? (
                                                <>
                                                    {/* The kind in words, as plain text; an unknown kind's code only for an administrator. */}
                                                    <span className="text-fg-muted" data-site-kind={d.project_type || 'php'}>
                                                        {siteKindWord(t, d.project_type, { admin: isAdmin })}
                                                    </span>
                                                    {(d.project_type || 'php') === 'php' && d.php_version && (
                                                        <span className="ml-1.5 text-xs text-fg-subtle">{d.php_version}</span>
                                                    )}
                                                </>
                                            ) : '—'}
                                        </td>
                                        <td className="whitespace-nowrap px-4 py-3 text-right text-fg-muted max-sm:basis-full max-sm:p-0 max-sm:text-left max-sm:before:mr-2 max-sm:before:text-fg-subtle max-sm:before:content-[attr(data-label)]" data-label={t('domains.col.disk')}>
                                            {canView(d, 'statistics') ? sizeWord(d.disk_usage, locale) : '—'}
                                        </td>
                                        <td className="whitespace-nowrap px-4 py-3 text-right text-fg-muted max-sm:basis-full max-sm:p-0 max-sm:text-left max-sm:before:mr-2 max-sm:before:text-fg-subtle max-sm:before:content-[attr(data-label)]" data-label={t('domains.col.traffic')}>
                                            {canView(d, 'statistics') ? t('domains.perMonth', { size: sizeWord(d.bandwidth, locale) }) : '—'}
                                        </td>
                                        <td className="px-4 py-3 max-sm:basis-full max-sm:p-0">
                                            {beingDeleted(d.id) ? (
                                                <span className="inline-flex items-center gap-1.5 text-fg-muted" data-domain-row-deleting="">
                                                    <Spinner size="xs" />
                                                    {t('domains.status.beingDeleted')}
                                                </span>
                                            ) : (
                                                <StateLabel status={d.status} admin={isAdmin} className="text-fg-muted" />
                                            )}
                                        </td>
                                        <td className="px-4 py-3 max-sm:basis-full max-sm:p-0 max-sm:pt-2">
                                            {/* Manage first, Open site beside it, Delete last under
                                                More, in red (P3, D1/X01). A domain without a
                                                website has nothing to open. */}
                                            <RowActions
                                                name={d.domain_name}
                                                actions={canView(d, 'files') && d.project_type !== 'dnsonly' ? [manageAction(d), visitAction(d)] : [manageAction(d)]}
                                                more={isTeamMember ? [] : [{
                                                    id: 'delete',
                                                    label: t('domains.action.delete'),
                                                    icon: Trash2,
                                                    danger: true,
                                                    onSelect: () => handleDelete(d.id, d.domain_name),
                                                    disabled: shown.stale || beingDeleted(d.id),
                                                    disabledReason: beingDeleted(d.id) ? t('domains.status.beingDeleted') : t('common.staleNotice'),
                                                }]}
                                            />
                                        </td>
                                    </tr>
                                ))}
                                {/* The infrastructure zone: DNS only. No selection, no
                                    visit, no removal; its one action opens the records.
                                    Altyapi bolgesi: yalniz DNS; tek eylemi kayitlari acar. */}
                                {infrastructureRows.map((zone) => (
                                    <tr key={`infrastructure-${zone.zone}`} className="border-b border-border last:border-0 hover:bg-surface-2/60 max-sm:flex max-sm:flex-wrap max-sm:items-start max-sm:gap-x-3 max-sm:gap-y-1.5 max-sm:p-4" data-infrastructure-zone={zone.zone}>
                                        {!isTeamMember && <td className="px-4 py-3 max-sm:hidden" />}
                                        <td className="px-4 py-3 max-sm:min-w-0 max-sm:flex-1 max-sm:p-0">
                                            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                                                <span className="inline-flex min-w-0 items-center gap-2">
                                                    <Network className="h-4 w-4 shrink-0 text-fg-subtle" aria-hidden="true" />
                                                    <button
                                                        onClick={() => navigate(infrastructureZonePath(zone.zone))}
                                                        className="text-left text-base font-medium text-primary hover:underline"
                                                    >
                                                        {zone.zone}
                                                    </button>
                                                </span>
                                                <span className="rounded-md border border-border bg-surface-2 px-1.5 py-0.5 text-xs font-medium text-fg-muted">
                                                    {t('domains.infrastructure.badge')}
                                                </span>
                                            </div>
                                        </td>
                                        <td className="px-4 py-3 text-fg-subtle max-sm:hidden">—</td>
                                        <td className="px-4 py-3 text-right text-fg-subtle max-sm:hidden">—</td>
                                        <td className="px-4 py-3 text-right text-fg-subtle max-sm:hidden">—</td>
                                        <td className="px-4 py-3 text-fg-muted max-sm:basis-full max-sm:p-0">
                                            {t('domains.infrastructure.status')}
                                            <span className="block text-xs text-fg-subtle" data-no-website="">{t('domains.infrastructure.noWebsite')}</span>
                                        </td>
                                        <td className="px-4 py-3 max-sm:basis-full max-sm:p-0 max-sm:pt-2">
                                            {/* Its records first. The zone has no website: the
                                                second action gives it one with the name filled in,
                                                behind the same gate as Add domain (the dialog also
                                                refuses until DNS can publish it).
                                                Once kayitlari; bolgenin web sitesi yok, ikinci
                                                eylem adi dolu olarak bir tane ekler. */}
                                            <RowActions
                                                name={zone.zone}
                                                actions={isTeamMember ? [zoneManage(zone.zone)] : [zoneManage(zone.zone), {
                                                    id: 'host-website',
                                                    label: t('domains.infrastructure.hostWebsite'),
                                                    icon: Plus,
                                                    onSelect: () => openAdd(zone.zone),
                                                    disabled: Boolean(dnsBlocked),
                                                    disabledReason: dnsRequirementText ?? undefined,
                                                }]}
                                            />
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>

                    <div className="border-t border-border px-4 py-2.5 text-xs text-fg-subtle">
                        {(listedCount === 1 ? t('common.itemsTotalOne') : t('common.itemsTotal', { n: listedCount }))}
                    </div>
                </div>
                ))}
            </RemoteGate>

            {!isTeamMember && showAddModal && (
                <AddDomainModal
                    initialDomain={addName}
                    sslByDefault={addServedHere}
                    onClose={() => setShowAddModal(false)}
                    onSuccess={(outcome) => {
                        setShowAddModal(false);
                        // The dialog already told every reader of domains;
                        // this page keeps what the create said about names
                        // that already point elsewhere (D-024).
                        const records = outcome?.siteRecords;
                        const name = outcome?.domain ?? addName;
                        setKept(records && records.kept.length > 0 && name ? { domain: name, outcome: records } : null);
                        changed();
                    }}
                />
            )}
        </div>
    );
}



// The title of a subscription card. The workspace the Panel made for an
// administrator is stored under an internal name; the administrator sees it as
// their own sites, never as "Admin Subscription".
// Yoneticinin kendi calisma alani ic bir adla saklanir; "Admin Subscription" gorunmez.
export function subscriptionTitle(s: { name: string; owner: string }, viewer: string | undefined, t: (key: TranslationKey) => string): string {
    return s.name === 'Admin Subscription' && (!viewer || s.owner === viewer) ? t('quota.ownWorkspace') : s.name;
}

// A compact usage strip: the caller's subscription(s) with measured disk
// against the plan limit and resource counts. Real numbers straight from
// the quota system that gates creation — what you see is what's enforced.
// Kompakt kullanım şeridi: çağıranın aboneliği/leri, plan limitine karşı
// ölçülen disk ve kaynak sayıları. Oluşturmayı kapılayan kota sisteminden
// gelen gerçek sayılar — gördüğün, uygulanandır.
function SubscriptionUsage() {
    const { t, locale } = useI18n();
    const { user } = useAuth();
    const viewer = user?.username;
    const usage = useRemote(SUBSCRIPTIONS_URL, decodeSubscriptions);

    // The strip exists only for subscriptions that carry measured usage, so
    // while that is being read there is nothing to announce. A failed read is
    // said in one line with Retry rather than drawn as "no usage".
    // Şerit yalnız ölçülmüş kullanımı olan abonelikler için vardır; okunurken
    // duyurulacak bir şey yoktur. Başarısız okuma "kullanım yok" diye
    // çizilmez; Tekrar dene ile tek satırda söylenir.
    if (usage.remote.state === 'loading') return null;
    if (usage.remote.state === 'unknown') {
        return (
            <CouldNotCheck
                className="mb-5"
                text={t('quota.unknown')}
                onRetry={() => void usage.retry()}
                busy={usage.reading}
            />
        );
    }
    const subs = usage.remote.value;

    const withUsage = subs.filter((s) => s.usage);
    if (withUsage.length === 0) return null;

    return (
        <div className="mb-5 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {withUsage.map((s) => {
                const u = s.usage!;
                const unlimited = u.disk_limit_bytes <= 0;
                const pct = unlimited ? 0 : Math.min(100, (u.disk_used_bytes / u.disk_limit_bytes) * 100);
                return (
                    <div key={s.id} className="rounded-xl border border-border bg-surface p-4">
                        <div className="mb-2 flex items-center gap-2">
                            <HardDrive className="h-4 w-4 text-primary" />
                            <span className="truncate text-base font-semibold text-fg">{subscriptionTitle(s, viewer, t)}</span>
                        </div>
                        {s.owner && s.owner !== viewer && <p className="-mt-1 mb-2 text-xs text-fg-subtle" data-subscription-owner="">{t('quota.accountOf', { owner: s.owner })}</p>}
                        {unlimited ? (
                            <p className="text-sm text-fg-muted">
                                {sizeWord(u.disk_used_bytes, locale)} · {t('quota.unlimited')}
                            </p>
                        ) : (
                            <>
                                <UsageBar percent={pct} />
                                <p className="mt-1.5 text-xs text-fg-muted">
                                    {t('quota.diskOf', { used: sizeWord(u.disk_used_bytes, locale), total: sizeWord(u.disk_limit_bytes, locale) })}
                                </p>
                            </>
                        )}
                        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-0.5 text-xs text-fg-subtle">
                            {/* A limit of 0 is unlimited (alpha.114), written ∞. */}
                            <span>{u.domains_limit > 0 ? t('quota.domains', { n: u.domains, max: u.domains_limit }) : t('quota.domainsFree', { n: u.domains })}</span>
                            <span>{u.databases_limit > 0 ? t('quota.databases', { n: u.databases, max: u.databases_limit }) : t('quota.databasesFree', { n: u.databases })}</span>
                            <span>{u.mail_limit > 0 ? t('quota.mail', { n: u.mail_accounts, max: u.mail_limit }) : t('quota.mailFree', { n: u.mail_accounts })}</span>
                        </div>
                    </div>
                );
            })}
        </div>
    );
}
