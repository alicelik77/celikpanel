import { lazy, Suspense, useState } from 'react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Link } from '../router';
import { Button } from './ui';
import type { SiteRecordsOutcome } from './AddDomainModal';

// The end of server setup (v0.1.0-alpha.115, owner-approved draft): an
// optional "Add your first site" with the main domain filled in. It opens the
// existing Add domain form (AddDomainModal), which says what can be hosted
// here and creates nothing until the owner submits it; "Later" only hides the
// offer. Which of the two the owner chose is a per-viewer convenience kept in
// this browser (and nothing breaks when the browser refuses to keep it).
// Kurulum sonunda istege bagli ilk site: mevcut Alan adi ekle formu, ad dolu.

const AddDomainModal = lazy(() => import('./AddDomainModal').then(module => ({ default: module.AddDomainModal })));

type Choice = 'offer' | 'added' | 'later';

const storageKey = (domain: string) => `celikpanel.setup.firstSite.${domain}`;

function readChoice(domain: string): Choice {
    try {
        const value = window.localStorage.getItem(storageKey(domain));
        return value === 'added' || value === 'later' ? value : 'offer';
    } catch {
        return 'offer';
    }
}

function keepChoice(domain: string, choice: Choice) {
    try {
        window.localStorage.setItem(storageKey(domain), choice);
    } catch {
        // A convenience only: the offer simply shows again next time.
    }
}

export function SetupFirstSite({ domain }: { domain: string }) {
    const { t: translate } = useI18n();
    const t = (key: string, values?: Record<string, string | number>) => translate(key as TranslationKey, values);
    const [choice, setChoice] = useState<Choice>(() => readChoice(domain));
    const [open, setOpen] = useState(false);
    // Names that already point elsewhere, as the create answered (this view only).
    const [kept, setKept] = useState<SiteRecordsOutcome | null>(null);
    const choose = (next: Choice) => { setChoice(next); keepChoice(domain, next); };
    if (choice !== 'offer') {
        return <section aria-labelledby="setup-first-site-title" className="space-y-3 rounded-lg border border-border bg-surface p-4 sm:p-5" data-setup-first-site={choice}>
            <h2 id="setup-first-site-title" className="font-semibold">{t('dnsov.site.title')}</h2>
            <p role="status" className="text-sm leading-6">{t(choice === 'added' ? 'dnsov.site.added' : 'dnsov.site.skipped')}</p>
            {choice === 'added' && kept && kept.kept.length > 0 && <div className="space-y-2 rounded-md border border-warning-mark/50 bg-warning-mark/10 p-4 text-sm leading-6" data-setup-first-site-kept="">
                <h3 className="font-semibold">{t('dnsov.site.keptTitle')}</h3>
                <dl className="grid gap-x-4 gap-y-1 sm:grid-cols-[minmax(0,9rem)_minmax(0,1fr)]">
                    <dt className="text-fg-muted">{t('setup.v2.why')}</dt><dd className="min-w-0 break-words">{t('dnsov.site.keptWhy', { names: kept.kept.map(row => `${row.name} ${row.type} ${row.content}`).join(', ') })}</dd>
                    <dt className="text-fg-muted">{t('setup.v2.who')}</dt><dd>{t('dnsov.site.keptWho')}</dd>
                    <dt className="text-fg-muted">{t('setup.v2.next')}</dt><dd className="min-w-0 break-words">{t('dnsov.site.keptNext', { ip: kept.ipv4 })}</dd>
                    <dt className="text-fg-muted">{t('setup.v2.then')}</dt><dd>{t('dnsov.site.keptThen')}</dd>
                </dl>
                <Link to={`/domains/${encodeURIComponent(domain)}?tab=dns`} className="inline-flex font-medium text-primary underline underline-offset-4">{t('dnsov.own.open')}</Link>
            </div>}
            <Link to="/dns-infrastructure" className="inline-flex text-sm font-medium text-primary underline underline-offset-4">{t('dnsov.site.openDNS')}</Link>
        </section>;
    }
    return <section aria-labelledby="setup-first-site-title" className="space-y-3 rounded-lg border border-border bg-surface p-4 sm:p-5" data-setup-first-site="offer">
        <h2 id="setup-first-site-title" className="font-semibold">{t('dnsov.site.title')} <span className="text-sm font-normal text-fg-muted">({t('dnsov.site.optional')})</span></h2>
        <p className="text-sm leading-6">{t('dnsov.site.why', { domain })}</p>
        <p className="text-sm leading-6 text-fg-muted">{t('dnsov.site.help')}</p>
        <div className="flex flex-wrap gap-3">
            <Button type="button" variant="primary" onClick={() => setOpen(true)}>{t('dnsov.site.add')}</Button>
            <Button type="button" variant="secondary" onClick={() => choose('later')}>{t('dnsov.site.later')}</Button>
        </div>
        {open && <Suspense fallback={null}>
            <AddDomainModal initialDomain={domain} onClose={() => setOpen(false)} onSuccess={(outcome) => { setOpen(false); setKept(outcome?.siteRecords ?? null); choose('added'); }} />
        </Suspense>}
    </section>;
}
