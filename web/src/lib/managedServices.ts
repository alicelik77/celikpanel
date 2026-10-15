import { decodeManagedServicesSnapshot, type ManagedServicesSnapshot } from '../components/ComponentOperation';
import { useCallback, useEffect, useSyncExternalStore } from 'react';
import { readApiError } from './apiError';
import { publishComponentCensus } from './componentCensus';
import { startVisibleReads } from './visibleReads';
import { mapRemote, useRemote, type Remote, type RemoteHandle } from './remote';

// The stored component records, for a screen that only shows them or takes a
// component's own facts from them: GET /api/v1/managed-services. One address,
// one decoder (the fail-closed one the operation tracker exports, plus the
// shape of the one field read here that it does not look at), one request
// shared by everything on screen. It reads the stored scan and never probes
// the host. A payload the decoder refuses is unknown; it is not "nothing
// installed", not "not checked yet" and not "this component has no
// configuration files": before 9 Oct 2026 the PostgreSQL and MariaDB pages
// said "postgresql.conf not found" on an installed server for as long as the
// read took, and for good when it failed.
//
// Every reader of this address uses `decodeManagedServices` (see
// lib/remote.ts: the first reader's decoder is the address's decoder). The
// answer is kept as the server sent it, so a reader takes what it needs with
// `mapRemote`.
//
// Kayıtlı bileşen kayıtları; onları yalnız gösteren ya da bir bileşenin kendi
// olgularını onlardan alan ekran için. Tek adres, tek çözücü (işlem
// izleyicinin dışa açtığı güvenli taraf çözücüsü ve onun bakmadığı, burada
// okunan tek alanın biçimi), ekrandaki her şeyin paylaştığı tek istek. Kayıtlı
// taramayı okur, makineyi asla yoklamaz. Çözücünün reddettiği yük
// bilinmeyendir; "hiçbir şey kurulu değil", "henüz bakılmadı" ya da "bu
// bileşenin yapılandırma dosyası yok" değildir.
export const MANAGED_SERVICES_URL = '/api/v1/managed-services';

// `config_files` is the list a reader here walks. The server writes a list or
// `null` (a Go list nobody appended to); anything else is not the contract.
// `config_files`, buradaki okuyucunun gezdiği listedir. Sunucu liste ya da
// `null` yazar; başka her şey sözleşme değildir.
function configFilesReadable(service: Record<string, unknown>): boolean {
    const files = service.config_files;
    if (files === undefined || files === null) return true;
    return Array.isArray(files) && files.every((file) => (
        !!file && typeof file === 'object' && typeof (file as { path?: unknown }).path === 'string'
    ));
}

export function decodeManagedServices(raw: unknown): ManagedServicesSnapshot {
    const snapshot = decodeManagedServicesSnapshot(raw);
    if (!snapshot || !snapshot.services.every(configFilesReadable)) throw new Error('shape');
    return snapshot;
}

// A panel that mounts within this time of the page's own read uses the page's
// answer and sends nothing: a component's page draws its sections once its
// header knows the component, a moment after the page itself asked. Without
// this that late section asked a second time, and a second answer that failed
// took away what the first had just shown.
// Sayfanın okumasından bu süre içinde açılan bölüm sayfanın yanıtını kullanır
// ve istek göndermez. Bu olmadan geç açılan bölüm ikinci kez soruyordu ve
// başarısız ikinci yanıt, ilkinin az önce gösterdiğini geri alıyordu.
const FRESH_FOR_MS = 30_000;

export function useManagedServices(): RemoteHandle<ManagedServicesSnapshot> {
    return useRemote(MANAGED_SERVICES_URL, decodeManagedServices, { freshFor: FRESH_FOR_MS });
}

// The configuration files the scan found for one component. An empty list is a
// known answer: the scan was read and names none for it. It is the same read
// as `useManagedServices`, so a page that shows both causes one request.
// Taramanın bir bileşen için bulduğu yapılandırma dosyaları. Boş liste bilinen
// bir yanıttır. `useManagedServices` ile aynı okumadır; ikisini gösteren sayfa
// tek istek yapar.
export function useComponentConfigFiles(serviceId: string): { files: Remote<string[]>; retry: () => void; reading: boolean } {
    const { remote, retry, reading } = useManagedServices();
    const files = mapRemote(remote, (snapshot) => {
        const record = snapshot.services.find((service) => service.id === serviceId);
        return ((record?.config_files as { path: string }[] | null | undefined) ?? [])
            .map((file) => file.path)
            .filter((path) => path !== '');
    });
    return { files, retry: () => void retry(), reading };
}

// --- A stored record the server cannot verify --------------------------------
//
// The stored component record is refused with 503 SERVICE_STATE_UNVERIFIED
// when the server cannot vouch for it. The Dashboard and the Components page
// answer that code with their own conditional check (D-032, 2026-10-15); a
// component's own page does the same instead of showing "could not be read"
// whose Retry kept failing until one of those pages ran the check. The check is
// POST /api/v1/managed-services/scan?max_age_seconds=300, a read that writes a
// new record: once per refusal, one request however many readers are on
// screen, no toast, and no repeat by itself. A failed check is the screen's
// existing "could not check" state; the person's Retry asks once more.
// Sunucunun doğrulayamadığı kayıtlı kayıt 503 SERVICE_STATE_UNVERIFIED ile
// reddedilir. Pano ve Bileşenler sayfası bunu kendi koşullu kontrolüyle
// yanıtlar; bir bileşenin kendi sayfası da artık aynısını yapar. Kontrol, her
// ret için bir kez, ekranda kaç okuyan olursa olsun tek istek, bildirimsiz ve
// kendiliğinden tekrarsızdır. Başarısız kontrol, ekranın mevcut "kontrol
// edilemedi" durumudur; Tekrar dene bir kez daha sorar.
export const STORED_RECORD_CHECK_URL = '/api/v1/managed-services/scan?max_age_seconds=300';
const STORED_RECORD_HELD_CODES = new Set(['server_setup_busy', 'service_operation_busy']);

type StoredRecordCheckState = 'idle' | 'checking' | 'done' | 'held' | 'failed';

let checkState: StoredRecordCheckState = 'idle';
let checkInflight: Promise<ManagedServicesSnapshot | null> | null = null;
const checkListeners = new Set<() => void>();
// The refusals a check was already started for, so a second reader of the same
// answer starts nothing.
// Kontrolü zaten başlatılmış retler; aynı yanıtın ikinci okuyanı bir şey başlatmaz.
const checkedRefusals = new WeakSet<object>();

function setCheckState(next: StoredRecordCheckState) {
    if (checkState === next) return;
    checkState = next;
    syncHeldPoll();
    for (const listener of [...checkListeners]) listener();
}

// --- A check held by a change resumes by itself (15 Oct 2026) ------------------
//
// The refusal "a change holds the server" (409 server_setup_busy or
// service_operation_busy) is not a failure of the check, and asking again at
// once would only be refused again. While a screen shows it, the page reads
// GET /api/v1/host-mutation-readiness (reads only, never a change) and, when
// that read goes from busy to ready, asks for the record once more: one
// follow-up check, as the Dashboard makes after its own held check. The read
// runs only while the tab is visible, one at a time, every 5 seconds, and
// stops with the held state; it is one poll however many readers are on screen.
// A change the poll never sees as busy leaves the person's Retry as it was.
// Bir değişiklik sunucuyu tuttuğu için reddedilen kontrol, değişiklik bitince
// kendiliğinden bir kez sürer: hazırlık okuması (yalnız GET) meşgulden hazıra
// geçince kayıt bir kez daha istenir. Okuma yalnız sekme görünürken, tek tek ve
// 5 saniyede bir yapılır; durum bitince durur; ekranda kaç okuyan olursa olsun
// tek yoklamadır. Yoklamanın meşgul görmediği değişiklikte Tekrar dene kalır.
const HELD_POLL_MS = 5000;
const rereaders = new Set<() => Promise<unknown>>();
let stopHeldPoll: (() => void) | null = null;

async function hostBusyNow(): Promise<boolean | null> {
    try {
        const response = await fetch('/api/v1/host-mutation-readiness', { method: 'GET', cache: 'no-store' });
        // An unreadable answer is no information: neither busy nor ready.
        if (!response.ok) throw new Error('unreadable');
        const payload = await response.json() as { ready?: unknown } | null;
        return payload && typeof payload.ready === 'boolean' ? !payload.ready : null;
    } catch {
        return null;
    }
}

function syncHeldPoll() {
    const wanted = checkState === 'held' && rereaders.size > 0;
    if (!wanted) {
        if (stopHeldPoll) { stopHeldPoll(); stopHeldPoll = null; }
        return;
    }
    if (stopHeldPoll) return;
    let sawBusy = false;
    let resumed = false;
    stopHeldPoll = startVisibleReads(async () => {
        const busy = await hostBusyNow();
        if (resumed || checkState !== 'held' || busy === null) return;
        if (busy) {
            sawBusy = true;
            return;
        }
        if (!sawBusy) return;
        resumed = true;
        // The record is read again (the one shared read); it is refused as
        // before and the check runs once. Leaving 'held' ends this poll.
        const [reread] = rereaders;
        setCheckState('idle');
        if (reread) void reread();
    }, HELD_POLL_MS, {
        setInterval: (callback, ms) => setInterval(callback, ms) as unknown as number,
        clearInterval: (id) => clearInterval(id),
    }, { immediate: typeof document === 'undefined' || document.visibilityState !== 'hidden' });
}

/** Forgets an episode (a check that never answered would otherwise be shared for good). For tests. */
export function resetStoredRecordCheck(): void {
    checkInflight = null;
    checkState = 'idle';
    syncHeldPoll();
}

export function isStoredRecordUnverified(remote: Remote<unknown>): boolean {
    return remote.state === 'unknown' && remote.reason.code === 'SERVICE_STATE_UNVERIFIED';
}

/** One conditional check at a time; everyone who asks meanwhile shares it. */
export function runStoredRecordCheck(): Promise<ManagedServicesSnapshot | null> {
    if (checkInflight) return checkInflight;
    setCheckState('checking');
    const pending = (async (): Promise<ManagedServicesSnapshot | null> => {
        try {
            const response = await fetch(STORED_RECORD_CHECK_URL, { method: 'POST', cache: 'no-store' });
            if (!response.ok) {
                const refusal = await readApiError(response);
                const held = response.status === 409 && !!refusal.code && STORED_RECORD_HELD_CODES.has(refusal.code);
                setCheckState(held ? 'held' : 'failed');
                return null;
            }
            const snapshot = decodeManagedServices(await response.json());
            // The check answers for the whole host: the sidebar badge takes it
            // from here.
            publishComponentCensus(snapshot.services);
            setCheckState('done');
            return snapshot;
        } catch {
            setCheckState('failed');
            return null;
        }
    })().finally(() => { checkInflight = null; });
    checkInflight = pending;
    return pending;
}

export interface StoredRecordCheck {
    /** The record was refused as unverifiable and the check is on its way (or about to start). */
    checking: boolean;
    /** The check was refused because a change holds the server. */
    held: boolean;
    /** Reads the record again; it also allows one more check. */
    retry: () => Promise<Remote<ManagedServicesSnapshot>>;
}

// A stable function: useSyncExternalStore resubscribes when it changes, which
// would look like the last reader leaving.
// Kararlı bir işlev: değişirse yeniden abone olunur ve bu, son okuyanın
// ayrılması gibi görünürdü.
function subscribeToCheck(listener: () => void): () => void {
    checkListeners.add(listener);
    return () => {
        checkListeners.delete(listener);
        // With no reader left on screen the episode is over: a later visit
        // starts from a clean state and checks once.
        // Ekranda okuyan kalmayınca bölüm biter; sonraki ziyaret temiz başlar.
        if (checkListeners.size === 0 && checkState !== 'checking') {
            checkState = 'idle';
            syncHeldPoll();
        }
    };
}

export function useStoredRecordCheck(handle: RemoteHandle<ManagedServicesSnapshot>): StoredRecordCheck {
    const state = useSyncExternalStore(subscribeToCheck, () => checkState);
    const { remote, retry: reread } = handle;
    const unverified = isStoredRecordUnverified(remote);

    // While this reader is on screen, a held check can resume through it.
    useEffect(() => {
        rereaders.add(reread);
        syncHeldPoll();
        return () => {
            rereaders.delete(reread);
            syncHeldPoll();
        };
    }, [reread]);

    useEffect(() => {
        if (remote.state === 'known' && checkState !== 'checking') setCheckState('idle');
        if (!unverified || checkedRefusals.has(remote)) return;
        checkedRefusals.add(remote);
        // After a failed, held or finished check only the person's Retry (or a
        // known answer) allows another: a refusal that returns after a
        // successful check is not asked about again by itself.
        // Başarısız, tutulmuş ya da biten bir kontrolden sonra yalnız kişinin
        // Tekrar dene'si (ya da bilinen bir yanıt) yenisine izin verir.
        if (checkState !== 'idle') return;
        void runStoredRecordCheck().then((snapshot) => { if (snapshot) void reread(); });
    }, [remote]);

    const retry = useCallback((): Promise<Remote<ManagedServicesSnapshot>> => {
        if (checkState !== 'checking') setCheckState('idle');
        return reread();
    }, [reread]);

    // Until the effect has started the check the refusal is already "being
    // checked", so the generic failure never flashes. After a successful check
    // the page is reading again.
    // Efekt kontrolü başlatana dek ret zaten "kontrol ediliyor"dur.
    const checking = unverified && (
        state === 'checking'
        || (state === 'idle' && !checkedRefusals.has(remote as object))
        || (state === 'done' && handle.reading)
    );
    return { checking, held: unverified && state === 'held', retry };
}

export type { ManagedServicesSnapshot };
