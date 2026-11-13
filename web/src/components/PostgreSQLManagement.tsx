import { Suspense, lazy, useState } from 'react';
import { Database, Settings, ShieldCheck, FileCode, type LucideIcon } from 'lucide-react';
import { ServiceShell } from './ServiceShell';
import { RepoCard } from './RepoCard';
import { PostgreSQLClustersCard } from './PostgreSQLClusters';
import { ComponentPanels } from './ComponentDetail';
import { ConfigEditor } from './ConfigEditor';
import { PostgreSQLSettings } from './PostgreSQLSettings';
import { PostgreSQLAccessRules } from './PostgreSQLAccessRules';
import { useI18n } from '../i18n';
import { useComponentConfigFiles } from '../lib/managedServices';
import { Checking, CouldNotCheck } from './ui';
import { editorCardHeight } from './ConfigFileNotices';
// alpha.133: the account CelikPanel connects with (drawing B19), in its own chunk.
const EngineAccountBlock = lazy(() => import('./DatabaseRemoteServerPanel').then((module) => ({ default: module.EngineAccountBlock })));

interface PostgreSQLManagementProps {
    onBack: () => void;
}

// PostgreSQL on ServiceShell. The shell owns status + start/stop; the body is
// real config editing: visual settings (postgresql.conf), access rules
// (pg_hba.conf), and raw file editing. Config files come from managed-services.
//
// Which files PostgreSQL has on this server is read from the server, and it is
// one of three things (9 Oct 2026): being read, could not be read, or known.
// "postgresql.conf not found" is said only for a scan that was read and does
// not name the file; before, it stood on an installed server for as long as
// the read took and for good when it failed.
//
// PostgreSQL, ServiceShell üzerinde. Durum + başlat/durdur kabuğa aittir;
// gövde gerçek yapılandırma düzenlemesidir. PostgreSQL'in bu sunucuda hangi
// dosyaları olduğu sunucudan okunur ve üç şeyden biridir: okunuyor, okunamadı
// ya da biliniyor. "postgresql.conf bulunamadı" yalnız okunmuş ve dosyayı
// adlandırmayan bir tarama için söylenir.
//
// alpha.130: with several PostgreSQL clusters (Debian/Ubuntu's
// /etc/postgresql/<version>/<name>/), a compact choice above the tabs says
// whose postgresql.conf and pg_hba.conf the editors open; the highest version
// first. With one cluster, or Arch's single data directory, nothing extra is
// shown. The choice lives in this screen only.
// Birden çok kümede düzenleyicilerin hangi kümenin dosyalarını açtığı
// sekmelerin üstünde seçilir; varsayılan en yüksek sürümdür.
const CLUSTER_FILE = /^\/etc\/postgresql\/([^/]+)\/([^/]+)\/(postgresql\.conf|pg_hba\.conf)$/;

export interface PGClusterFiles {
    version: string;
    name: string;
    conf?: string;
    hba?: string;
}

/** The clusters named by the file paths, highest version first. */
export function pgClustersFromFiles(paths: string[]): PGClusterFiles[] {
    const found = new Map<string, PGClusterFiles>();
    for (const path of paths) {
        const match = CLUSTER_FILE.exec(path);
        if (!match) continue;
        const [, version, name, file] = match;
        const key = `${version}/${name}`;
        const entry = found.get(key) ?? { version, name };
        if (file === 'postgresql.conf') entry.conf = path;
        else entry.hba = path;
        found.set(key, entry);
    }
    return [...found.values()].sort((a, b) => (Number(b.version) - Number(a.version)) || b.version.localeCompare(a.version) || (a.name === 'main' ? -1 : b.name === 'main' ? 1 : a.name.localeCompare(b.name)));
}

export function PostgreSQLManagement({ onBack }: PostgreSQLManagementProps) {
    const { t } = useI18n();
    const { files, retry, reading } = useComponentConfigFiles('postgresql');
    const [tab, setTab] = useState<'visual' | 'access'>('visual');
    const [rawFile, setRawFile] = useState<string | null>(null);
    const [picked, setPicked] = useState<string | null>(null);

    if (rawFile) {
        return <ConfigEditor path={rawFile} onBack={() => setRawFile(null)} />;
    }

    const known = files.state === 'known' ? files.value : null;
    const clusters = known ? pgClustersFromFiles(known) : [];
    const several = clusters.length > 1;
    const chosen = several ? clusters.find((cluster) => `${cluster.version}/${cluster.name}` === picked) ?? clusters[0] : null;
    const mainConf = chosen ? chosen.conf : known?.find((path) => path.endsWith('postgresql.conf'));
    const hbaConf = chosen ? chosen.hba : known?.find((path) => path.endsWith('pg_hba.conf'));

    return (
        <ServiceShell serviceId="postgresql" name="PostgreSQL" icon={Database} onBack={onBack}>
            {several && chosen && (
                <div className="mb-3 flex flex-wrap items-center gap-2" data-pg-cluster-pick="">
                    <span className="text-xs font-medium text-fg-subtle" id="pg-cluster-pick-label">{t('pgc.pick.label')}</span>
                    <div role="group" aria-labelledby="pg-cluster-pick-label" className="inline-flex flex-wrap rounded-lg border border-border bg-surface p-0.5">
                        {clusters.map((cluster) => {
                            const ref = `${cluster.version}/${cluster.name}`;
                            const active = cluster === chosen;
                            return (
                                <button
                                    key={ref}
                                    type="button"
                                    onClick={() => setPicked(ref)}
                                    aria-pressed={active}
                                    data-pg-cluster-choice={ref}
                                    className={`min-h-[2.25rem] rounded-md px-3 py-1.5 text-sm font-medium transition-colors max-sm:min-h-11 ${active ? 'bg-primary/10 text-primary' : 'text-fg-muted hover:text-fg'}`}
                                >
                                    {t('pgc.pick.cluster', { version: cluster.version, name: cluster.name })}
                                </button>
                            );
                        })}
                    </div>
                </div>
            )}
            <div className="mb-4 flex items-center gap-1 border-b border-border">
                <Tab active={tab === 'visual'} onClick={() => setTab('visual')} icon={Settings} label={t('db.tab.visual')} />
                <Tab active={tab === 'access'} onClick={() => setTab('access')} icon={ShieldCheck} label={t('db.tab.access')} />
            </div>

            <div className={`rounded-xl border border-border bg-surface p-5 ${editorCardHeight}`}>
                {files.state === 'loading' ? (
                    <Checking label={t('dbconf.files.checking', { service: 'PostgreSQL' })} className="min-h-[2.75rem] py-2" />
                ) : !known ? (
                    <CouldNotCheck text={t('dbconf.files.unknown', { service: 'PostgreSQL' })} onRetry={retry} busy={reading} />
                ) : tab === 'visual' ? (
                    mainConf ? (
                        <PostgreSQLSettings key={mainConf} configPath={mainConf} />
                    ) : (
                        <p className="py-8 text-center text-sm text-fg-muted">{t('db.fileNotFound', { file: 'postgresql.conf' })}</p>
                    )
                ) : hbaConf ? (
                    <PostgreSQLAccessRules key={hbaConf} configPath={hbaConf} />
                ) : (
                    <p className="py-8 text-center text-sm text-fg-muted">{t('db.fileNotFound', { file: 'pg_hba.conf' })}</p>
                )}
            </div>

            {known && known.length > 0 && (
                <div className="mt-6">
                    <h4 className="mb-2 text-xs font-semibold uppercase tracking-wider text-fg-muted">{t('db.rawFiles')}</h4>
                    <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
                        {known.map((path) => (
                            <button
                                key={path}
                                onClick={() => setRawFile(path)}
                                className="flex min-h-[2.75rem] items-center gap-2.5 rounded-lg border border-border bg-surface px-3 py-2 text-left transition-colors hover:bg-surface-2"
                            >
                                <FileCode className="h-4 w-4 shrink-0 text-fg-muted" aria-hidden="true" />
                                <span className="truncate font-mono text-xs text-fg-muted">{path}</span>
                            </button>
                        ))}
                    </div>
                </div>
            )}
            {/* Overview + journal; the config list is skipped — this page
                has a real editor for those files already. / Genel bakış +
                günlük; ayar listesi atlanır — bu sayfada o dosyalar için
                gerçek bir editör zaten var.
                They are drawn from the same read as the file list above, so
                they come with its answer: while that read is on its way or
                has failed, the card above says so once, with one Retry.
                Üstteki dosya listesiyle aynı okumadan çizilirler; o okuma
                sürerken ya da başarısız olduğunda bunu üstteki kart bir kez,
                tek bir Tekrar dene ile söyler. */}
            <div className="mt-6 empty:hidden"><Suspense fallback={null}><EngineAccountBlock engine="postgresql" /></Suspense></div>
            {/* alpha.108: every major as its own cluster on its own port. */}
            <div className="mt-6"><PostgreSQLClustersCard /></div>
            {/* alpha.107: the PGDG repository's card (D-038 §12). */}
            <div className="mt-6 empty:hidden"><RepoCard serviceId="postgresql" /></div>
            {known && <ComponentPanels serviceId="postgresql" show={{ configs: false }} />}
        </ServiceShell>
    );
}

function Tab({ active, onClick, icon: Icon, label }: { active: boolean; onClick: () => void; icon: LucideIcon; label: string }) {
    return (
        <button
            onClick={onClick}
            aria-pressed={active}
            className={`-mb-px flex min-h-[2.75rem] items-center gap-2 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors ${
                active ? 'border-primary text-primary' : 'border-transparent text-fg-muted hover:text-fg'
            }`}
        >
            <Icon className="h-4 w-4" aria-hidden="true" />
            {label}
        </button>
    );
}
