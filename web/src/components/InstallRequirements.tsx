import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import type { RemoteHandle } from '../lib/remote';
import type { InstallNeed, InstallPlan } from '../lib/installPlan';
import { Checking, CouldNotCheck, KeepWords, StatusDot, type StatusDotTone } from './ui';

// The install window's "Requires" (alpha.124, D-045 follow-up): what the
// component needs, at component level, each with its state - installed,
// installed together with it first (one operation on the server), or missing
// and not installable from here with the reason and where it is done. The
// PHP tools choose the installed PHP-FPM version they run on here. Package
// names never appear; the repository change the owner must consent to is
// said below by InstallPreviewNotes.
// Kurulum penceresinin "Gerekenler" bölümü: bileşen düzeyinde, durumu ve
// nedeniyle.

export interface RequirementComponent { id: string; name: string; versions?: string[] }

// One dot per state, the panel's own status mark: installed (green), installed
// together (hollow: not here yet), missing (red), not known (grey).
const stateTone: Record<InstallNeed['state'], StatusDotTone> = { installed: 'success', together: 'idle', unavailable: 'danger', unknown: 'unknown' };

export function InstallRequirements({ name, handle, components, roleName, phpVersions, phpVersion, phpDefault, onPHPVersion, busy }: {
    /** The component being installed, as the page names it. */
    name: string;
    handle: RemoteHandle<InstallPlan>;
    components: readonly RequirementComponent[];
    /** A role's name ("Web server"), or null when the token is a component. */
    roleName: (token: string) => string | null;
    /** The installed PHP-FPM versions a PHP tool may run on, newest first; [] for any other component. */
    phpVersions: readonly string[];
    phpVersion: string;
    /** The distribution's own PHP version, when it is known. */
    phpDefault: string;
    onPHPVersion: (version: string) => void;
    busy: boolean;
}) {
    const { t } = useI18n();
    const remote = handle.remote;
    if (remote.state === 'loading') return <Checking label={t('cinst.checking', { name })} className="mb-4" />;
    if (remote.state !== 'known') {
        return <CouldNotCheck className="mb-4" text={t('cinst.unknownPlan', { name })} onRetry={() => void handle.retry()} busy={handle.reading} />;
    }
    const plan = remote.value;
    const nameOf = (id: string) => components.find((item) => item.id === id)?.name ?? id;
    const versionOf = (id: string) => components.find((item) => item.id === id)?.versions?.[0] ?? '';
    const said = (need: InstallNeed) => {
        const component = nameOf(need.component);
        switch (need.state) {
        case 'installed': {
            const version = need.component === 'php-fpm' ? '' : versionOf(need.component);
            return version ? t('cinst.installedVersion', { version }) : t('cinst.installed');
        }
        case 'together':
            return t('cinst.together', { name });
        case 'unknown':
            return t('cinst.unknownNeed');
        default:
            return t(`cinst.reason.${need.reason ?? 'elsewhere'}` as TranslationKey, { name, component, holder: need.holder ? nameOf(need.holder) : '' });
        }
    };
    const phpNeed = phpVersions.length > 0 ? plan.requires.find((need) => need.need === 'php-fpm' && need.state === 'installed' && !need.for) : undefined;
    const togetherNames = plan.together.map(nameOf);
    return (
        <section className="mb-4 rounded-lg border border-border bg-surface-2/50 p-3 text-sm" data-install-requires={plan.requires.map((need) => `${need.need}:${need.state}`).join(' ')}>
            <h3 className="mb-2 text-xs font-medium text-fg-subtle">{t('cinst.requiresTitle')}</h3>
            {plan.blocked && (
                <p className="mb-2 rounded-md border border-danger/40 bg-danger/10 px-2.5 py-2 text-fg" role="alert" data-install-blocked="">
                    <KeepWords text={t('cinst.blocked', { name })} />
                </p>
            )}
            {plan.requires.length === 0
                ? <p className="text-fg-muted" data-install-requires-none="">{t('cinst.nothing')}</p>
                : (
                    <ul className="space-y-2">
                        {plan.requires.map((need) => {
                            const role = roleName(need.need);
                            const label = role
                                ? (need.component ? `${role}: ${nameOf(need.component)}` : role)
                                : nameOf(need.need);
                            return (
                                <li key={`${need.for ?? ''}|${need.need}`} className="flex items-start gap-2" data-install-need={need.need} data-install-need-state={need.state}>
                                    <span className="mt-1.5 flex shrink-0" aria-hidden="true"><StatusDot tone={stateTone[need.state]} /></span>
                                    <div className="min-w-0 flex-1">
                                        <p className="font-medium text-fg">
                                            <KeepWords text={label} />
                                            {need.for && <span className="font-normal text-fg-muted"> {t('cinst.forOther', { name: nameOf(need.for) })}</span>}
                                        </p>
                                        <p className={need.state === 'unavailable' || need.state === 'unknown' ? 'text-fg' : 'text-fg-muted'}>
                                            <KeepWords text={said(need)} />
                                        </p>
                                        {need === phpNeed && (
                                            <div className="mt-1.5" data-install-php="">
                                                <p className="mb-1 text-xs font-medium text-fg-subtle">{t('cinst.phpTitle', { name })}</p>
                                                <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label={t('cinst.phpTitle', { name })}>
                                                    {phpVersions.map((version) => (
                                                        <button
                                                            key={version}
                                                            type="button"
                                                            role="radio"
                                                            aria-checked={phpVersion === version}
                                                            onClick={() => onPHPVersion(version)}
                                                            disabled={busy}
                                                            data-install-php-version={version}
                                                            className={`rounded-md px-2.5 py-1 font-mono text-xs font-medium transition-colors max-sm:min-h-11 ${
                                                                phpVersion === version ? 'bg-primary text-primary-fg' : 'bg-surface-3 text-fg-muted hover:text-fg'
                                                            } disabled:cursor-not-allowed disabled:opacity-50`}
                                                        >
                                                            {version === phpDefault ? t('cinst.phpDefault', { version }) : `PHP ${version}`}
                                                        </button>
                                                    ))}
                                                </div>
                                                <p className="mt-1 text-xs text-fg-muted">{t('cinst.phpHint', { name })}</p>
                                            </div>
                                        )}
                                    </div>
                                </li>
                            );
                        })}
                    </ul>
                )}
            {!plan.blocked && togetherNames.length > 0 && (
                <p className="mt-2 text-fg-muted" data-install-together={plan.together.join(' ')}>
                    <KeepWords text={t('cinst.togetherSummary', { names: togetherNames.join(', '), name })} />
                </p>
            )}
        </section>
    );
}
