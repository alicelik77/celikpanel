import { useCallback, useEffect, useState } from 'react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { componentName } from '../lib/componentName';
import { decodeSetupComponentCatalog, setupComponentClosure, setupComponentGroups, type SetupComponent, type SetupComponentCatalog } from '../lib/serverSetupComponents';
import { Button, KeepWords } from './ui';

// The last catalog this page read. The wizard remounts when the language
// changes; without this the step list guessed from a missing catalog for a
// moment (an extra "Components" step that then vanished). A fresh read still
// replaces it. / Dil degisince sihirbaz yeniden kurulur; son katalog korunur.
let rememberedCatalog: SetupComponentCatalog | null = null;

export function useSetupComponentCatalog(enabled: boolean) {
    const [catalog, setCatalog] = useState<SetupComponentCatalog | null>(rememberedCatalog);
    const [failed, setFailed] = useState(false);
    const [attempt, setAttempt] = useState(0);
    const reload = useCallback(() => setAttempt(value => value + 1), []);
    useEffect(() => {
        if (!enabled) return;
        let alive = true;
        const controller = new AbortController();
        const timeout = window.setTimeout(() => controller.abort(), 20000);
        setFailed(false);
        void fetch('/api/v1/setup/components', { cache: 'no-store', signal: controller.signal }).then(async response => {
            if (!response.ok) throw new Error('catalog unavailable');
            const next = decodeSetupComponentCatalog(await response.json());
            if (!next) throw new Error('catalog invalid');
            rememberedCatalog = next;
            if (alive) setCatalog(next);
        }).catch(() => { if (alive) setFailed(true); }).finally(() => window.clearTimeout(timeout));
        return () => { alive = false; controller.abort(); window.clearTimeout(timeout); };
    }, [enabled, attempt]);
    return { catalog, failed, reload };
}

const categories: Record<string, TranslationKey> = {
    web: 'setup.components.category.web', database: 'setup.components.category.database',
    email: 'setup.components.category.email', cache: 'setup.components.category.cache',
    security: 'setup.components.category.security', ftp: 'setup.components.category.ftp',
    monitoring: 'setup.components.category.monitoring', system: 'setup.components.category.system',
    dns: 'setup.components.category.dns',
};
const categoryOrder = ['web', 'database', 'email', 'cache', 'security', 'ftp', 'monitoring', 'system', 'dns'];

// Why a row the wizard does not offer as a choice is shown without a box.
function rowReason(row: SetupComponent): TranslationKey | null {
    const code = row.reason?.split(':')[0] || '';
    if (row.wizard === 'dns') return 'setup.components.reason.dnsStep';
    if (row.wizard === 'automatic') return 'setup.components.reason.automatic';
    if (row.wizard === 'unavailable' || code === 'server_setup_service_integration') return 'setup.components.reason.integration';
    if (code === 'server_setup_repository_required') return 'setup.components.reason.repository';
    if (row.wizard === 'components') return 'setup.components.reason.afterSetup';
    return null;
}

// The components grid (setup wizard v2): every catalogue component, grouped by
// category. Choices have a box; an installed component shows "Installed ·
// version" instead and is never removed; the others say why they are not a
// choice here. A selected component with versions shows its version and, on
// request, the alternatives: the distribution's (recommended) and a vendor's
// from an enabled repository, or a locked line saying where to enable it.
export function ServerSetupComponents({ catalog, selected, onChange, disabled, versions = {}, onVersions }: {
    catalog: SetupComponentCatalog;
    selected: string[];
    onChange: (selected: string[]) => void;
    disabled: boolean;
    versions?: Record<string, string>;
    onVersions?: (versions: Record<string, string>) => void;
}) {
    const { t } = useI18n();
    const [open, setOpen] = useState<string | null>(null);
    const wizard = (row: SetupComponent) => row.wizard ?? (catalog.required_components.includes(row.id) ? 'required' : 'choice');
    const effective = setupComponentClosure(selected, catalog);
    const choiceRows = catalog.components.filter(row => wizard(row) === 'choice');
    const groups = setupComponentGroups({ ...catalog, components: choiceRows });
    const lookup = new Map(catalog.components.map(row => [row.id, row]));
    const unknown = selected.filter(id => { const row = lookup.get(id); return !row || wizard(row) !== 'choice'; });
    const ready = catalog.inventory_state === 'ready';
    const toInstall = [...effective].filter(id => !(ready && lookup.get(id)?.installed));
    const kept = ready ? catalog.components.filter(row => row.installed).length : 0;
    const name = (id: string) => componentName(lookup.get(id) ?? id, t);
    const system = catalog.host?.distro_id ? `${catalog.host.distro_id}${catalog.host.version_id ? ` ${catalog.host.version_id}` : ''}` : '';
    const otherRows = (category: string) => catalog.components.filter(row => row.category === category && wizard(row) !== 'choice' && wizard(row) !== 'required');
    const shown = categoryOrder.filter(category => groups.some(group => group[0].category === category) || otherRows(category).length > 0);
    for (const category of new Set(catalog.components.map(row => row.category))) if (!shown.includes(category) && (groups.some(group => group[0].category === category) || otherRows(category).length > 0)) shown.push(category);
    const versionText = (row: SetupComponent) => {
        const chosen = versions[row.id];
        if (chosen) {
            const entry = row.versions?.find(item => item.package === chosen);
            return t('setup.components.versionVendor', { version: entry?.version || chosen, repo: row.repo?.name || '' });
        }
        const version = row.candidate_version || row.versions?.find(item => item.source === 'distribution')?.version;
        return version ? t('setup.components.versionDistribution', { version }) : t('setup.components.versionDistributionUnknown');
    };
    return <fieldset disabled={disabled} className="space-y-4">
        <legend className="text-xl font-semibold">{t('setup.components.title')}</legend>
        <p className="pt-2 text-sm leading-6 text-fg-muted">{t('setup.components.help')}</p>
        <p className="flex flex-wrap gap-x-5 gap-y-1 text-sm" data-setup-components-summary="">
            <span>{t('setup.components.summary', { count: toInstall.length })}</span>
            {kept > 0 && <span>{t('setup.components.summaryKept', { count: kept })}</span>}
            {system && <span className="text-fg-muted">{t('setup.components.summaryHost', { system })}</span>}
        </p>
        <p className="text-sm leading-6 text-fg-muted">{t('setup.components.catalogueNote')}</p>
        {catalog.inventory_state === 'unknown' && <p role="alert" className="text-sm text-danger">{t('setup.components.inventoryUnknown')}</p>}
        {unknown.length > 0 && <div role="alert" className="space-y-2 text-sm text-danger"><p>{t('setup.components.savedUnavailable')}</p><Button type="button" onClick={() => onChange(selected.filter(id => { const row = lookup.get(id); return !!row && wizard(row) === 'choice'; }))}>{t('setup.components.removeUnavailable')}</Button></div>}
        <div className="grid items-start gap-5 xl:grid-cols-2">
        {shown.map(category => <fieldset key={category} className="min-w-0 rounded-lg border border-border bg-surface">
            <legend className="ml-4 px-1 text-base font-semibold">{t(categories[category] || 'setup.components.category.other')}</legend>
            <div className="divide-y divide-border">
                {groups.filter(group => group[0].category === category).map(group => {
                    const groupIDs = new Set(group.map(row => row.id));
                    const checked = group.some(row => effective.has(row.id));
                    const neededBy = selected.filter(id => !groupIDs.has(id) && group.some(row => setupComponentClosure([id], catalog).has(row.id)));
                    const conflicts = catalog.components.filter(row => (effective.has(row.id) || (ready && row.installed)) && !groupIDs.has(row.id)
                        && group.some(member => row.conflicts.includes(member.id) || member.conflicts.includes(row.id)));
                    const unavailable = !ready || group.some(row => !row.supported);
                    const installed = ready && group.every(row => row.installed);
                    const locked = neededBy.length > 0 || (!checked && (unavailable || conflicts.length > 0));
                    const label = group.map(row => componentName(row, t)).join(' + ');
                    const rowID = `setup-component-${group[0].id}`;
                    const versioned = group.length === 1 && checked && !installed && (group[0].versions?.length || 0) > 0 ? group[0] : null;
                    const detail = installed ? t('setup.components.installedKeep')
                        : neededBy.length > 0 ? t('setup.components.requiredBy', { names: neededBy.map(name).join(', ') })
                        : unavailable ? t('setup.components.unavailable')
                            : conflicts.length > 0 ? t('setup.components.conflicts', { names: conflicts.map(row => componentName(row, t)).join(', ') })
                                : group.length > 1 ? t('setup.components.together')
                                    : group[0].dependencies.length > 0 ? t('setup.components.includes', { names: group[0].dependencies.map(name).join(', ') })
                                        : t('setup.components.optional');
                    return <div key={rowID} className="px-4 py-3" data-component-row={group[0].id}>
                        <div className="flex items-start gap-3">
                            {installed
                                ? <span className="mt-0.5 w-4 shrink-0 text-center text-success" aria-hidden="true">✓</span>
                                : <input id={rowID} type="checkbox" checked={checked} disabled={locked} aria-describedby={`${rowID}-detail`}
                                    onChange={() => onChange(checked ? selected.filter(id => !groupIDs.has(id)) : [...selected, group[0].id])}
                                    className="mt-1 h-4 w-4 shrink-0 accent-primary" />}
                            <span className="min-w-0 flex-1">
                                <span className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
                                    {installed ? <span className="font-medium">{label}</span> : <label htmlFor={rowID} className={`font-medium ${locked ? 'cursor-default' : 'cursor-pointer'}`}>{label}</label>}
                                    {installed && <span className="text-xs text-fg-muted" data-installed-label="">{group.map(row => row.installed_version).filter(Boolean).length > 0 ? t('setup.components.installedVersion', { version: group.map(row => row.installed_version).filter(Boolean).join(' · ') }) : t('setup.components.installed')}</span>}
                                </span>
                                {/* "PHP-FPM" and "E-posta" do not break at their hyphen. */}
                                <span id={`${rowID}-detail`} className="mt-1 block text-sm leading-6 text-fg-muted"><KeepWords text={detail} /></span>
                                {versioned && <span className="mt-1 block text-sm" data-component-version={versioned.id}>
                                    {t('setup.components.version')}: <span className="font-medium">{versionText(versioned)}</span>
                                    {onVersions && <button type="button" className="ml-2 font-medium text-primary underline underline-offset-4" aria-expanded={open === versioned.id} onClick={() => setOpen(open === versioned.id ? null : versioned.id)}>{t(open === versioned.id ? 'setup.components.versionClose' : 'setup.components.versionChange')}</button>}
                                </span>}
                                {versioned && open === versioned.id && onVersions && <span role="radiogroup" aria-label={t('setup.components.version')} className="mt-2 block space-y-2 rounded-md border border-border bg-surface-2 p-3 text-sm">
                                    {versioned.versions!.map((item, index) => item.available
                                        ? <label key={`${item.source}-${item.package}-${index}`} className="flex cursor-pointer items-start gap-2">
                                            <input type="radio" name={`setup-version-${versioned.id}`} value={item.package} checked={(versions[versioned.id] || '') === item.package}
                                                onChange={() => { const next = { ...versions }; if (item.package) next[versioned.id] = item.package; else delete next[versioned.id]; onVersions(next); }}
                                                className="mt-1 h-4 w-4 shrink-0 accent-primary" />
                                            <span>{item.source === 'distribution'
                                                ? (item.version ? t('setup.components.versionDistribution', { version: item.version }) : t('setup.components.versionDistributionUnknown'))
                                                : <>{t('setup.components.versionVendor', { version: item.version || item.package, repo: versioned.repo?.name || '' })}<span className="block text-xs text-fg-muted">{t('setup.components.versionVendorNote')}</span></>}</span>
                                        </label>
                                        : <span key={`locked-${index}`} className="block text-fg-muted" data-version-locked={item.reason}>
                                            <KeepWords text={t(item.reason === 'repository_disabled' ? 'setup.components.versionLocked' : 'setup.components.versionUnknownRepo', { repo: versioned.repo?.name || '', name: componentName(versioned, t) })} />
                                        </span>)}
                                    {versioned.id === 'php-fpm' && <span className="block text-xs text-fg-muted">{t('setup.components.versionPHPTools')}</span>}
                                </span>}
                            </span>
                        </div>
                    </div>;
                })}
                {otherRows(category).map(row => {
                    const reason = rowReason(row);
                    const installed = ready && row.installed;
                    return <div key={row.id} className="px-4 py-3" data-component-row={row.id} data-wizard={row.wizard}>
                        <div className="flex items-start gap-3">
                            <span className={`mt-0.5 w-4 shrink-0 text-center ${installed ? 'text-success' : 'text-fg-muted'}`} aria-hidden="true">{installed ? '✓' : '–'}</span>
                            <span className="min-w-0 flex-1">
                                <span className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1"><span className="font-medium">{componentName(row, t)}</span>
                                    {installed && <span className="text-xs text-fg-muted">{row.installed_version ? t('setup.components.installedVersion', { version: row.installed_version }) : t('setup.components.installed')}</span>}</span>
                                <span className="mt-1 block text-sm leading-6 text-fg-muted"><KeepWords text={installed && row.wizard !== 'automatic' ? t('setup.components.installedKeep') : reason ? t(reason) : t('setup.components.unavailable')} /></span>
                            </span>
                        </div>
                    </div>;
                })}
            </div>
        </fieldset>)}
        </div>
        <p className="border-t border-border pt-4 text-sm leading-6 text-fg-muted" data-setup-always="">{t('setup.components.always')}</p>
        <p className="text-sm leading-6 text-fg-muted">{t('setup.components.preserve')}</p>
    </fieldset>;
}
