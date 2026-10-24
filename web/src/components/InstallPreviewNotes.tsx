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
import { notableGroups, type InstallPreview } from '../lib/installPreview';
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
    const notable = remote.value.notable.map((item) => item.package);
    if (notable.length === 0) return { hold: false, confirmed: [] };
    return acknowledged ? { hold: false, confirmed: notable } : { hold: true, confirmed: [] };
}

export function InstallPreviewNotes({
    handle,
    acknowledged,
    onAcknowledge,
    componentLabel,
    avoidable,
}: {
    handle: RemoteHandle<InstallPreview>;
    acknowledged: boolean;
    onAcknowledge: (value: boolean) => void;
    /** A catalogue id said as the page names it. */
    componentLabel: (id: string) => string;
    /** A version other than the recommended one was chosen. */
    avoidable: boolean;
}) {
    const { t } = useI18n();
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
    if (preview.notable.length === 0) {
        return (
            <p className="mt-3 text-sm leading-6 text-fg-muted" data-install-preview="clean">
                {t('services.installPreview.nothingElse')}
            </p>
        );
    }
    const groups = notableGroups(preview.notable);
    return (
        <div className="mt-3 rounded-lg border border-warning-mark/50 bg-warning-mark/20 p-3 text-sm leading-6 text-fg" role="status" data-install-preview="notable">
            <p className="flex items-start gap-2 font-medium">
                <AlertTriangle className="mt-1 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                {t('services.installPreview.notableTitle')}
            </p>
            <ul className="mt-1.5 space-y-1.5 pl-6">
                {groups.map((group) => (
                    <li key={group.key} data-notable-reason={group.reason}>
                        <KeepWords
                            text={t(`services.installPreview.reason.${group.reason}` as TranslationKey, {
                                version: group.version ?? '',
                                component: group.component ? componentLabel(group.component) : '',
                                packages: group.packages.join(', '),
                            })}
                        />
                    </li>
                ))}
            </ul>
            {avoidable && <p className="mt-1.5 pl-6 text-fg-muted">{t('services.installPreview.avoid')}</p>}
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
