// What an install brings, said in its confirmation before anything changes
// (alpha.105, D-034 amendment: recommend, warn, never impose). The package
// manager's simulation is read while the dialogue is open, with visible
// waiting; notable packages are named with a plain reason and need the
// owner's explicit confirmation; a read that fails says so truthfully and
// that the server checks the install again before it runs.
// Bir kurulumun getirdikleri, hiçbir şey değişmeden önce onayında söylenir.
import { AlertTriangle } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Checking, CouldNotCheck, KeepWords } from './ui';
import { capitalize, confirmationGroups, distroWords, hasVendorReplacement, reasonPhrase, versionPair, versionsSaid, type InstallPreview, type NotableInstall } from '../lib/installPreview';
import type { RemoteHandle } from '../lib/remote';

// installPreviewGate: what the confirmation may send. A known answer with
// notable packages holds Install until they are confirmed; the confirmed
// names go with the install so the server can check them again. A read in
// flight holds Install; a read that failed never does (the server checks).
// Onayın ne gönderebileceği.
export function installPreviewGate(handle: RemoteHandle<InstallPreview> | null, acknowledged: boolean): { hold: boolean; confirmed: string[] } {
    if (!handle) return { hold: false, confirmed: [] };
    const remote = handle.remote;
    if (remote.state === 'loading' || handle.reading) return { hold: true, confirmed: [] };
    if (remote.state !== 'known' || remote.value.state !== 'known') return { hold: false, confirmed: [] };
    // A vendor replacement holds Install whatever is ticked (alpha.107).
    if (hasVendorReplacement(remote.value.notable)) return { hold: true, confirmed: [] };
    const notable = remote.value.notable.map((item) => item.package);
    if (notable.length === 0) return { hold: false, confirmed: [] };
    return acknowledged ? { hold: false, confirmed: notable } : { hold: true, confirmed: [] };
}

// installPreviewAckKey: what a tick confirms, the read's address and the
// notable names it showed, sorted (alpha.105 review). A tick never carries
// over to another answer for the same address.
// Bir onayın neyi onayladığı: okumanın adresi ve gösterdiği adlar.
export function installPreviewAckKey(url: string | null, handle: RemoteHandle<InstallPreview> | null): string | null {
    if (!url || !handle || handle.remote.state !== 'known' || handle.remote.value.state !== 'known') return null;
    return `${url}|${handle.remote.value.notable.map((item) => item.package).sort().join(',')}`;
}

export function InstallPreviewNotes({
    handle,
    acknowledged,
    onAcknowledge,
    componentLabel,
    avoidable,
    selected,
    recommended,
}: {
    handle: RemoteHandle<InstallPreview>;
    acknowledged: boolean;
    onAcknowledge: (value: boolean) => void;
    /** A catalogue id said as the page names it. */
    componentLabel: (id: string) => string;
    /** A version other than the distribution's own was chosen. */
    avoidable: boolean;
    /** What the owner chose, as the dialog names it ("PHP-FPM 8.5"). */
    selected?: string;
    /** The distribution's own version, said short, when it is known. */
    recommended?: string;
}) {
    const { t, locale } = useI18n();
    const remote = handle.remote;
    if (remote.state === 'loading') {
        return <Checking label={t('services.installPreview.checking')} className="mt-3" />;
    }
    const preview = remote.state === 'known' ? remote.value : null;
    if (!preview || preview.state !== 'known') {
        const why = preview?.state === 'busy' ? t('services.installPreview.whyBusy') : '';
        return (
            <CouldNotCheck
                className="mt-3"
                text={t('services.installPreview.unknown', { why })}
                onRetry={() => void handle.retry()}
                busy={handle.reading}
            />
        );
    }
    const existing = (preview.existingData ?? []).length > 0 && (
        <p className="mt-3 rounded-lg border border-border bg-surface-2 p-3 text-sm leading-6 text-fg" data-install-preview-existing="">
            <KeepWords text={t('services.installPreview.existingData', { paths: (preview.existingData ?? []).map((item) => item.path).join(', ') })} />
        </p>
    );
    if (preview.notable.length === 0) {
        return (
            <>
                {existing}
                <p className="mt-3 text-sm leading-6 text-fg-muted" data-install-preview="clean">
                    {t('services.installPreview.nothingElse')}
                </p>
            </>
        );
    }
    const say = (key: string, params?: Record<string, string>) => t(key as TranslationKey, params);
    const repoLabel = (id: string) => {
        const key = `services.notable.repoName.${id}` as TranslationKey;
        const said = id ? t(key) : key;
        return said === key ? id || t('services.notable.repoUnknown') : said;
    };
    const distro = distroWords(preview.distribution, locale, say);
    if (hasVendorReplacement(preview.notable)) {
        const refused = preview.notable.filter((item) => item.reason === 'vendor_replaces_distribution');
        const repos = [...new Set(refused.map((item) => item.component ?? ''))];
        return (
            <div className="mt-3 rounded-lg border border-danger/40 bg-danger/10 p-3 text-sm leading-6 text-fg" role="alert" data-install-preview="vendor-refused">
                <ul className="space-y-2">
                    {repos.map((repo) => (
                        <li key={repo} className="flex items-start gap-2" data-notable-reason="vendor_replaces_distribution">
                            <AlertTriangle className="mt-1 h-4 w-4 shrink-0 text-danger" aria-hidden="true" />
                            <div className="min-w-0 flex-1">
                                <KeepWords text={t('services.installPreview.vendorRefused', { repo: repoLabel(repo), ...distro })} />
                                <ul className="mt-1 space-y-0.5">
                                    {refused.filter((item) => (item.component ?? '') === repo).map((item) => (
                                        <li key={item.package} className="break-words" title={item.from && item.to ? `${item.package} ${item.from} → ${item.to}` : undefined} data-notable-package={item.package}>
                                            <strong className="font-semibold">{item.package}</strong>{versionsSaid(item)}
                                        </li>
                                    ))}
                                </ul>
                            </div>
                        </li>
                    ))}
                </ul>
                <p className="mt-1.5 pl-6 text-fg-muted">{t('services.installPreview.vendorRefusedWhy')}</p>
                <p className="mt-1.5 pl-6 text-fg-muted">{t('services.installPreview.vendorNext')}</p>
            </div>
        );
    }
    const { replaced, additional } = confirmationGroups(preview.notable);
    const chosen = selected || t('services.installPreview.selectedUnknown');
    const fullTitle = (item: NotableInstall) => (item.from && item.to ? `${item.package} ${item.from} → ${item.to}` : undefined);
    return (
        <div className="mt-3 min-w-0 rounded-lg border border-warning-mark/50 bg-warning-mark/20 p-3 text-sm leading-6 text-fg" role="status" data-install-preview="notable">
            <p className="flex items-start gap-2 font-medium">
                <AlertTriangle className="mt-1 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                <span className="min-w-0 break-words">{t('services.installPreview.heading', { selected: chosen })}</span>
            </p>
            <div className="mt-2 space-y-3 pl-6">
                {replaced.length > 0 && (
                    <section data-install-preview-group="replaced">
                        <h4 className="font-semibold">{t('services.installPreview.replacedTitle')}</h4>
                        {replaced.map((group) => {
                            const repo = repoLabel(group.repo);
                            const many = group.items.length > 1;
                            return (
                                <div key={group.repo} className="mt-1" data-notable-reason="vendor_shared">
                                    <ul className="space-y-1">
                                        {group.items.map((item) => (
                                            <li key={item.package} className="break-words" title={fullTitle(item)} data-notable-package={item.package}>
                                                <strong className="font-semibold">{item.package}</strong>
                                                {': '}
                                                {item.from && item.to
                                                    ? t('services.installPreview.replaced', { ...distro, repo, ...versionPair(item.from, item.to) })
                                                    : t('services.installPreview.replacedNoVersions', { ...distro, repo })}
                                            </li>
                                        ))}
                                    </ul>
                                    <p className="mt-1 text-fg-muted">{t(many ? 'services.installPreview.whyMany' : 'services.installPreview.whyOne', { selected: chosen, repo })}</p>
                                    <p className="text-fg-muted">{t(many ? 'services.installPreview.afterMany' : 'services.installPreview.afterOne', { ...distro, repo })}</p>
                                </div>
                            );
                        })}
                    </section>
                )}
                {additional.length > 0 && (
                    <section data-install-preview-group="additional">
                        <h4 className="font-semibold">{t('services.installPreview.additionalTitle')}</h4>
                        <ul className="mt-1 space-y-1.5">
                            {additional.map((group) => {
                                const phrase = reasonPhrase(group, say, componentLabel, repoLabel);
                                const noteKey = `services.installPreview.note.${group.reason}` as TranslationKey;
                                const note = t(noteKey);
                                return (
                                    <li key={group.key} className="break-words" data-notable-reason={group.reason}>
                                        {group.items.length === 1 ? (
                                            <span title={fullTitle(group.items[0])} data-notable-package={group.items[0].package}>
                                                <strong className="font-semibold">{group.items[0].package}</strong>
                                                {versionsSaid(group.items[0])}
                                                {': '}
                                                {phrase}
                                            </span>
                                        ) : (
                                            <>
                                                <span>{capitalize(phrase, locale)}:</span>
                                                <ul className="space-y-0.5 pl-3">
                                                    {group.items.map((item) => (
                                                        <li key={item.package} title={fullTitle(item)} data-notable-package={item.package}>
                                                            <strong className="font-semibold">{item.package}</strong>
                                                            {versionsSaid(item)}
                                                        </li>
                                                    ))}
                                                </ul>
                                            </>
                                        )}
                                        {note !== noteKey && <span className="block text-fg-muted">{note}</span>}
                                    </li>
                                );
                            })}
                        </ul>
                    </section>
                )}
                {avoidable && (
                    <p className="text-fg-muted" data-install-preview-avoid="">
                        {recommended
                            ? t('services.installPreview.avoid', { ...distro, recommended })
                            : t('services.installPreview.avoidNoVersion', distro)}
                    </p>
                )}
            </div>
            <label className="mt-2.5 flex cursor-pointer items-start gap-2.5 pl-6 max-sm:min-h-11">
                <input
                    type="checkbox"
                    checked={acknowledged}
                    onChange={(event) => onAcknowledge(event.target.checked)}
                    className="mt-1 h-4 w-4 shrink-0 accent-primary"
                    data-install-preview-confirm=""
                />
                <span>{t('services.installPreview.confirm')}</span>
            </label>
        </div>
    );
}
