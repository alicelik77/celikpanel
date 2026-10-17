import { useEffect, useState } from 'react';
import { DownloadCloud } from 'lucide-react';
import { Link } from '../router';
import { useI18n } from '../i18n';
import {
    availablePanelUpdate,
    snoozeUpdateNotice,
    updateNoticeSnoozed,
    usePanelUpdateDiscovery,
    wholeVersion,
} from '../lib/panelUpdateDiscovery';

// The Dashboard's one quiet row when the Panel found a newer signed release
// (D-035, owner request 2026-10-17). An update is not a problem, so it is not
// in "Needs attention" and carries no warning colour: neutral surface, the
// primary ink of a link, and words. "Remind me later" hides it for 24 hours
// in this browser; the sidebar link stays. Administrators only; a failed or
// unknown automatic check shows nothing here.
//
// Panonun tek sakin satiri: guncelleme bir sorun degildir, "Dikkat
// gerektirenler"de durmaz ve uyari rengi tasimaz. "Sonra hatirlat" bu
// tarayicida 24 saat gizler; kenar cubugu baglantisi kalir.
export function DashboardUpdateNotice({ admin }: { admin: boolean }) {
    const { t } = useI18n();
    const discovery = usePanelUpdateDiscovery(admin);
    const target = admin ? availablePanelUpdate(discovery.value) : null;
    const version = target?.version ?? '';
    const [snoozed, setSnoozed] = useState(() => version !== '' && updateNoticeSnoozed(version, Date.now()));
    useEffect(() => {
        setSnoozed(version !== '' && updateNoticeSnoozed(version, Date.now()));
    }, [version]);
    if (!target || snoozed) return null;
    return (
        <section
            aria-label={t('panelUpdate.notice.region')}
            data-dashboard-update={target.version}
            className="mb-6 flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border border-border bg-surface px-4 py-3 text-sm"
        >
            <p className="flex min-w-0 flex-1 basis-64 items-center gap-2 text-fg">
                <DownloadCloud className="h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
                <span>{wholeVersion(t('panelUpdate.notice.dashboard', { version: target.version }), target.version)}</span>
            </p>
            <span className="flex flex-wrap items-center gap-x-4 gap-y-1">
                <Link
                    to="/settings?section=updates"
                    className="inline-flex items-center font-semibold text-primary underline underline-offset-4 hover:decoration-2 max-sm:min-h-11"
                >
                    {t('panelUpdate.notice.review')}
                </Link>
                <button
                    type="button"
                    data-update-later=""
                    onClick={() => {
                        snoozeUpdateNotice(target.version, Date.now());
                        setSnoozed(true);
                    }}
                    className="inline-flex items-center text-fg-muted underline-offset-4 hover:text-fg hover:underline max-sm:min-h-11"
                >
                    {t('panelUpdate.notice.later')}
                </button>
            </span>
        </section>
    );
}
