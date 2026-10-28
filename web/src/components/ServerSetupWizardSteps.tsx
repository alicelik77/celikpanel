import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useI18n } from '../i18n';
import { setupDNSTranslation } from '../i18n/setupDNS';
import type { TranslationKey } from '../i18n/en';
import { componentName } from '../lib/componentName';
import { setupComponentClosure, setupRolePreset, type SetupComponentCatalog, type SetupDNSEngineChoice, type SetupRoles } from '../lib/serverSetupComponents';
import { mergeSetupSPF, setupDNSNames, setupImportedDMARC, setupImportedMX, setupImportedSPF, setupNameWithin, type ServerSetupDraft, type SetupImportedRecord } from '../lib/serverSetup';
import { useRefreshEvery, useRemote } from '../lib/remote';
import { Button, inputClass, KeepWords } from './ui';

// Setup wizard v2 (alpha.111): the editing steps and the guidance cards of the
// progress screen. The wizard itself (ServerSetup.tsx) owns the draft, the
// saved revision, the reviewed plan and the operation; these only render
// fields and say plainly what this release does.
// Sihirbaz v2 adimlari ve ilerleme kartlari; taslak ve islem ServerSetup'ta.

export function useSetupWizardI18n() {
    const i18n = useI18n();
    return { ...i18n, t: (key: TranslationKey, values?: Record<string, string | number>) => setupDNSTranslation(i18n.locale, key, values) ?? i18n.t(key, values) };
}

export function setupServerLabel(t: (key: TranslationKey, values?: Record<string, string | number>) => string, shortName?: string) {
    return shortName ? t('setup.v2.thisServerNamed', { name: shortName }) : t('setup.v2.thisServer');
}

export function SetupWizardInput({ name, label, value, onChange, placeholder, required, help, invalid }: { name: string; label: string; value: string; onChange: (value: string) => void; placeholder?: string; required?: boolean; help?: ReactNode; invalid?: string }) {
    return <div className="space-y-2">
        <label className="block" htmlFor={`setup-${name}`}><span className="mb-2 block text-sm font-medium">{label}</span><input id={`setup-${name}`} name={name} value={value} onChange={event => onChange(event.target.value)} placeholder={placeholder} required={required} autoCapitalize="none" autoComplete="off" spellCheck={false} aria-invalid={invalid ? true : undefined} aria-describedby={help || invalid ? `setup-${name}-help` : undefined} className={`${inputClass} w-full font-mono`} /></label>
        {(help || invalid) && <p id={`setup-${name}-help`} className={`text-sm leading-6 ${invalid ? 'text-danger' : 'text-fg-muted'}`}>{invalid || help}</p>}
    </div>;
}

function Choice({ name, value, checked, disabled, onChange, title, tag, help, extra, type = 'radio' }: { name: string; value: string; checked: boolean; disabled?: boolean; onChange: () => void; title: ReactNode; tag?: string; help: string; extra?: ReactNode; type?: 'radio' | 'checkbox' }) {
    return <label className={`flex items-start gap-3 rounded-lg border p-4 ${checked ? 'border-primary bg-surface-2' : 'border-border bg-surface'} ${disabled ? 'text-fg-muted' : 'cursor-pointer hover:bg-surface-2'}`}>
        <input type={type} name={name} value={value} checked={checked} disabled={disabled} onChange={onChange} className="mt-1 h-4 w-4 shrink-0 accent-primary" />
        <span className="min-w-0"><span className="flex flex-wrap items-center gap-2 font-semibold">{title}{tag && <span className="rounded-full bg-surface-subtle px-2 py-0.5 text-xs font-medium text-fg-muted">{tag}</span>}</span>
            <span className="mt-1 block text-sm leading-6 text-fg-muted"><KeepWords text={help} /></span>{extra}</span>
    </label>;
}

export function SetupDomainStep({ value, onChange, invalid }: { value: string; onChange: (value: string) => void; invalid: boolean }) {
    const { t } = useSetupWizardI18n();
    return <section className="space-y-5" aria-labelledby="setup-domain-title">
        <h2 id="setup-domain-title" className="text-xl font-semibold">{t('setup.v2.domain.title')}</h2>
        <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v2.domain.help', { example: `panel.${value || 'example.com'}` })}</p>
        <div className="max-w-md"><SetupWizardInput name="main_domain" label={t('setup.v2.domain.label')} value={value} onChange={onChange} placeholder="example.com" required help={t('setup.v2.domain.hint')} invalid={invalid ? t('setup.v2.domain.invalid') : undefined} /></div>
    </section>;
}

export function SetupCountStep({ value, onChange }: { value: 1 | 2; onChange: (value: 1 | 2) => void }) {
    const { t } = useSetupWizardI18n();
    return <fieldset className="space-y-4">
        <legend className="text-xl font-semibold">{t('setup.v2.count.title')}</legend>
        <div className="grid gap-3 pt-2">
            <Choice name="setup-server_count" value="1" checked={value === 1} onChange={() => onChange(1)} title={t('setup.v2.count.one')} help={t('setup.v2.count.one.help')} />
            <Choice name="setup-server_count" value="2" checked={value === 2} onChange={() => onChange(2)} title={t('setup.v2.count.two')} tag={t('setup.v2.recommended')} help={t('setup.v2.count.two.help')} />
        </div>
        {value === 2 && <p className="max-w-2xl text-sm leading-6 text-fg-muted" role="note">{t('setup.v2.peerIPNote')}</p>}
    </fieldset>;
}

export function SetupNameStep({ value, suggestion, mainDomain, onChange, invalid }: { value: string; suggestion?: string; mainDomain: string; onChange: (value: string) => void; invalid: boolean }) {
    const { t } = useSetupWizardI18n();
    return <section className="space-y-5" aria-labelledby="setup-name-title">
        <h2 id="setup-name-title" className="text-xl font-semibold">{t('setup.v2.name.title')}</h2>
        <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v2.name.help', { example: `${value || 'frankfurt'}.${mainDomain || 'example.com'}` })}</p>
        <div className="max-w-sm"><SetupWizardInput name="short_name" label={t('setup.v2.name.label')} value={value} onChange={onChange} placeholder="frankfurt"
            help={suggestion && value === suggestion ? t('setup.v2.name.suggested', { host: suggestion }) : t('setup.v2.name.optional')} invalid={invalid ? t('setup.v2.name.invalid') : undefined} /></div>
    </section>;
}

export function SetupRolesStep({ roles, onRoles, catalog, server, serverCount, customized, onCustomize, onReset, disabled }: {
    roles: SetupRoles; onRoles: (roles: SetupRoles) => void; catalog: SetupComponentCatalog | null; server: string; serverCount: 1 | 2;
    customized: boolean; onCustomize: () => void; onReset: () => void; disabled: boolean;
}) {
    const { t } = useSetupWizardI18n();
    const installs = (role: 'web' | 'mail') => catalog ? [...setupComponentClosure(setupRolePreset(role, catalog), catalog)].map(id => componentName(catalog.components.find(row => row.id === id) ?? id, t)).join(', ') : '…';
    const cards: { role: keyof SetupRoles; title: TranslationKey; help: TranslationKey; brings: string }[] = [
        { role: 'web', title: 'setup.v2.role.web', help: 'setup.v2.role.web.help', brings: installs('web') },
        { role: 'mail', title: 'setup.v2.role.mail', help: 'setup.v2.role.mail.help', brings: installs('mail') },
        { role: 'dns', title: 'setup.v2.role.dns', help: serverCount === 2 ? 'setup.v2.role.dns.helpTwo' : 'setup.v2.dns.localOneServer', brings: t('setup.v2.role.dnsInstalls') },
    ];
    return <fieldset className="space-y-4" disabled={disabled}>
        <legend className="text-xl font-semibold">{t('setup.v2.roles.title', { server })}</legend>
        <p className="max-w-2xl pt-2 text-sm leading-6 text-fg-muted">{t('setup.v2.roles.help')}</p>
        <div className="grid gap-3">{cards.map(card => <Choice key={card.role} type="checkbox" name="setup-role" value={card.role} checked={roles[card.role]} disabled={(!catalog && card.role !== 'dns') || (card.role === 'dns' && serverCount !== 2 && !roles.dns)}
            onChange={() => onRoles({ ...roles, [card.role]: !roles[card.role] })} title={t(card.title)} help={t(card.help)}
            extra={<span className="mt-2 block text-sm leading-6"><span className="font-medium">{t('setup.v2.role.installs')}</span> {card.brings}</span>} />)}</div>
        {customized
            ? <p className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm"><span>{t('setup.v2.roles.customized')}</span><Button type="button" variant="secondary" onClick={onCustomize}>{t('setup.v2.roles.seeComponents')}</Button><button type="button" className="font-medium text-primary underline underline-offset-4" onClick={onReset}>{t('setup.v2.roles.reset')}</button></p>
            : <p className="flex flex-wrap items-center gap-x-4 gap-y-2"><Button type="button" variant="secondary" disabled={!catalog} onClick={onCustomize}>{t('setup.components.customize')}</Button><span className="text-sm text-fg-muted">{t('setup.v2.roles.customizeHelp')}</span></p>}
        <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v2.roles.always')}</p>
    </fieldset>;
}

export function SetupNamesStep({ draft, mail, web, mainDomain, ip, ownDNS, onChange }: {
    draft: ServerSetupDraft; mail: boolean; web: boolean; mainDomain: string; ip: string; ownDNS: boolean;
    onChange: (key: 'panel_domain' | 'mail_hostname', value: string) => void;
}) {
    const { t } = useSetupWizardI18n();
    const outside = (value: string) => ownDNS && value.trim() !== '' && !setupNameWithin(value, mainDomain) ? t('setup.v2.names.outsideZone', { domain: mainDomain }) : undefined;
    return <section className="space-y-5" aria-labelledby="setup-names-title">
        <h2 id="setup-names-title" className="text-xl font-semibold">{t('setup.v2.names.title')}</h2>
        <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v2.names.help')}</p>
        <div className="grid max-w-2xl gap-5">
            <SetupWizardInput name="panel_domain" label={t('setup.v2.names.panel')} value={draft.panel_domain} onChange={value => onChange('panel_domain', value)} placeholder={`panel.${mainDomain || 'example.com'}`} required invalid={outside(draft.panel_domain)} />
            {mail && <SetupWizardInput name="mail_hostname" label={t('setup.v2.names.mail')} value={draft.mail_hostname} onChange={value => onChange('mail_hostname', value)} placeholder={`mail.${mainDomain || 'example.com'}`} required help={t('setup.v2.names.mailHelp', { domain: mainDomain || 'example.com' })} invalid={outside(draft.mail_hostname)} />}
        </div>
        {ip && <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v2.names.pointTo', { ip })}</p>}
        {web && mainDomain && <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v2.names.siteNote', { domain: mainDomain })}</p>}
    </section>;
}

const engineReason: Record<string, TranslationKey> = {
    server_setup_pdns_main_untested_system: 'setup.v2.engine.untested',
    server_setup_existing_dns_requires_migration: 'setup.v2.engine.existing',
    bind_source_pdns_switch_unsupported: 'setup.v2.engine.existing',
    pdns_primary_switch_paused: 'setup.v2.engine.paused',
    server_setup_dns_engine_unsupported: 'setup.v2.engine.unsupported',
    server_setup_inventory_unavailable: 'setup.v2.engine.unknown',
};

export function setupEngineChoice(catalog: SetupComponentCatalog | null, engine: 'bind' | 'pdns', role: 'primary' | 'secondary'): SetupDNSEngineChoice | undefined {
    return catalog?.dns_engines?.find(item => item.id === engine && item.role === (role === 'primary' ? 'main' : 'backup'));
}

export function SetupDNSHomeStep({ draft, set, onMode, onRole, catalog, serverCount, mainDomain, detectedIP, connection, publisher }: {
    draft: ServerSetupDraft;
    set: <K extends keyof ServerSetupDraft>(key: K, value: ServerSetupDraft[K]) => void;
    onMode: (mode: ServerSetupDraft['dns_mode']) => void;
    onRole: (role: ServerSetupDraft['dns_role']) => void;
    catalog: SetupComponentCatalog | null;
    serverCount: 1 | 2;
    mainDomain: string;
    detectedIP: string;
    connection: ReactNode;
    publisher: ReactNode;
}) {
    const { t } = useSetupWizardI18n();
    const names = setupDNSNames(draft);
    const primary = draft.dns_role === 'primary';
    const engine = (id: 'bind' | 'pdns') => setupEngineChoice(catalog, id, draft.dns_role);
    const engineHelp = (id: 'bind' | 'pdns') => {
        const choice = engine(id);
        if (choice && !choice.offered) return t(engineReason[choice.reason || ''] || 'setup.v2.engine.unsupported');
        if (id === 'bind') return t('setup.v2.dns.bind.help');
        return t(primary ? 'setup.v2.dns.pdns.helpMain' : 'setup.v2.dns.pdns.helpBackup');
    };
    const version = (id: 'bind' | 'pdns') => engine(id)?.installed_version || engine(id)?.candidate_version || '';
    const oneServer = serverCount === 1;
    const peerName = (value: string) => { set(names.peerKey, value); set('peer_ns', value); };
    return <section className="space-y-6" aria-labelledby="setup-dnshome-title">
        <div><h2 id="setup-dnshome-title" className="text-xl font-semibold">{t('setup.v2.dns.title')}</h2>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v2.dns.help', { domain: mainDomain || 'example.com' })}</p></div>
        <div className="grid gap-3">
            <Choice name="setup-dns" value="local" checked={draft.dns_mode === 'local'} disabled={oneServer && draft.dns_mode !== 'local'} onChange={() => onMode('local')} title={t('setup.v2.dns.local')} tag={oneServer ? undefined : t('setup.v2.recommended')} help={t(oneServer ? 'setup.v2.dns.localOneServer' : 'setup.v2.dns.local.help')} />
            <Choice name="setup-dns" value="external" checked={draft.dns_mode === 'external'} onChange={() => onMode('external')} title={t('setup.v2.dns.external')} help={t('setup.v2.dns.external.help')} />
            <Choice name="setup-dns" value="existing" checked={draft.dns_mode === 'existing'} onChange={() => onMode('existing')} title={t('setup.v2.dns.existing')} help={t('setup.v2.dns.existing.help')} />
        </div>
        {draft.dns_mode === 'local' && oneServer && <p role="alert" className="max-w-2xl text-sm leading-6 text-danger">{t('setup.v2.dns.localOneServer')}</p>}
        {draft.dns_mode === 'external' && <div className="rounded-lg border border-border bg-surface p-4 text-sm leading-6"><p className="font-semibold">{t('setup.v2.dns.withChoice')}</p>
            <ul className="mt-2 list-disc space-y-1 pl-5 text-fg-muted"><li>{t('setup.v2.dns.external.point1')}</li><li>{t('setup.v2.dns.external.point2')}</li><li>{t('setup.v2.dns.external.point3')}</li></ul></div>}
        {draft.dns_mode === 'existing' && connection}
        {draft.dns_mode === 'local' && !oneServer && <div className="space-y-6">
            <fieldset className="space-y-3"><legend className="font-semibold">{t('setup.v2.dns.role.title')}</legend>
                <div className="grid gap-3 lg:grid-cols-2">
                    <Choice name="setup-dns_role" value="primary" checked={primary} onChange={() => onRole('primary')} title={t('setup.v2.dns.role.main')} help={t('setup.v2.dns.role.main.help')} />
                    <Choice name="setup-dns_role" value="secondary" checked={!primary} onChange={() => onRole('secondary')} title={t('setup.v2.dns.role.backup')} help={t('setup.v2.dns.role.backup.help')} />
                </div>
            </fieldset>
            <div className="grid gap-6 rounded-xl border border-border bg-surface p-4 sm:p-5 lg:grid-cols-2 lg:gap-8">
                <fieldset className="min-w-0 space-y-4">
                    <legend className="mb-0 font-semibold">{t(primary ? 'setup.v2.dns.thisMain' : 'setup.v2.dns.thisBackup')}</legend>
                    <SetupWizardInput name={names.localKey} label={t('setup.v2.dns.serverName')} value={draft[names.localKey]} onChange={value => set(names.localKey, value)} placeholder={`${names.localKey}.${mainDomain || 'example.com'}`} required />
                    <SetupWizardInput name="local_ip" label={t('setup.v2.dns.ipv4')} value={draft.local_ip} onChange={value => set('local_ip', value)} required help={detectedIP && draft.local_ip === detectedIP ? t('setup.v2.dns.detected') : undefined} />
                </fieldset>
                <fieldset className="min-w-0 space-y-4 border-t border-border pt-5 lg:border-l lg:border-t-0 lg:pl-8 lg:pt-0">
                    <legend className="mb-0 font-semibold">{t(primary ? 'setup.v2.dns.peerBackup' : 'setup.v2.dns.peerMain')}</legend>
                    <SetupWizardInput name={names.peerKey} label={t('setup.v2.dns.serverName')} value={draft[names.peerKey]} onChange={peerName} placeholder={`${names.peerKey}.${mainDomain || 'example.com'}`} required />
                    <SetupWizardInput name="peer_ip" label={t('setup.v2.dns.ipv4')} value={draft.peer_ip} onChange={value => set('peer_ip', value)} required help={t('setup.v2.peerIPNote')} />
                </fieldset>
            </div>
            {primary && <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v2.dns.peerLater')}</p>}
            <fieldset className="space-y-3"><legend className="font-semibold">{t('setup.v2.dns.software')}</legend>
                <div className="grid gap-3 lg:grid-cols-2">{(['bind', 'pdns'] as const).map(id => {
                    const choice = engine(id);
                    const blocked = !!choice && !choice.offered;
                    return <Choice key={id} name="setup-dns_engine" value={id} checked={draft.dns_engine === id} disabled={blocked && draft.dns_engine !== id} onChange={() => set('dns_engine', id)}
                        title={<>{id === 'bind' ? 'BIND' : 'PowerDNS'}{version(id) && <span className="font-normal text-fg-muted">{version(id)}</span>}</>} tag={id === 'bind' ? t('setup.v2.recommended') : undefined} help={engineHelp(id)} />;
                })}</div>
                {engine(draft.dns_engine) && !engine(draft.dns_engine)!.offered && <p role="alert" className="text-sm leading-6 text-danger">{engineHelp(draft.dns_engine)}</p>}
                <p className="text-sm leading-6 text-fg-muted">{t('setup.v2.dns.oneSoftware')}</p>
            </fieldset>
            {primary ? <details className="rounded-lg border border-border bg-surface p-4 text-sm" open={!!(draft.infrastructure_dns?.peer_panel_domain || draft.infrastructure_dns?.peer_mail_hostname)}>
                <summary className="cursor-pointer font-medium text-primary">{t('setup.v2.dns.peerNames')}</summary>
                <div className="mt-3 space-y-4"><p className="leading-6 text-fg-muted">{t('setup.v2.dns.peerNamesHelp', { ip: draft.peer_ip || '—' })}</p>
                    <SetupWizardInput name="peer_panel_domain" label={t('setup.v2.dns.peerPanel')} value={draft.infrastructure_dns?.peer_panel_domain || ''} onChange={value => set('infrastructure_dns', { zone: mainDomain, ...draft.infrastructure_dns, peer_panel_domain: value })} placeholder={`boston.${mainDomain || 'example.com'}`} />
                    <SetupWizardInput name="peer_mail_hostname" label={t('setup.v2.dns.peerMail')} value={draft.infrastructure_dns?.peer_mail_hostname || ''} onChange={value => set('infrastructure_dns', { zone: mainDomain, ...draft.infrastructure_dns, peer_mail_hostname: value })} placeholder={`mail2.${mainDomain || 'example.com'}`} />
                </div>
            </details> : <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.infrastructure.secondaryHelp', { name: draft.panel_domain || '—', ip: draft.local_ip || '—', primary: draft.ns1 || '—' })}</p>}
            {publisher}
        </div>}
    </section>;
}

// What D-024 asks first: the reason, who acts, the next action and how the
// work resumes.
export function SetupLead({ title, why, who, next, then, tone = 'neutral' }: { title: string; why: ReactNode; who: string; next: ReactNode; then: string; tone?: 'neutral' | 'done' }) {
    const { t } = useSetupWizardI18n();
    return <div role="status" className={`rounded-lg border p-4 sm:p-5 ${tone === 'done' ? 'border-success/40 bg-success/5' : 'border-border bg-surface'}`}>
        <h3 className="font-semibold">{title}</h3>
        <dl className="mt-3 grid gap-x-4 gap-y-2 text-sm leading-6 sm:grid-cols-[minmax(0,9rem)_minmax(0,1fr)]">
            <dt className="text-fg-muted">{t('setup.v2.why')}</dt><dd className="min-w-0 break-words">{why}</dd>
            <dt className="text-fg-muted">{t('setup.v2.who')}</dt><dd className="min-w-0">{who}</dd>
            <dt className="text-fg-muted">{t('setup.v2.next')}</dt><dd className="min-w-0 break-words">{next}</dd>
            <dt className="text-fg-muted">{t('setup.v2.then')}</dt><dd className="min-w-0">{then}</dd>
        </dl>
    </div>;
}

export function SetupCopyRow({ label, value }: { label: string; value: string }) {
    const { t } = useSetupWizardI18n();
    const [copied, setCopied] = useState(false);
    const timer = useRef<number | undefined>(undefined);
    useEffect(() => () => window.clearTimeout(timer.current), []);
    // Copying is a convenience: when the browser refuses, the value stays
    // selectable and the button keeps its label.
    const copy = () => navigator.clipboard?.writeText(value).then(() => {
        setCopied(true); window.clearTimeout(timer.current); timer.current = window.setTimeout(() => setCopied(false), 2000);
    }, () => setCopied(false));
    return <div className="flex min-w-0 flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-surface-2 px-3 py-2 text-sm">
        <span className="min-w-0"><span className="block text-xs text-fg-muted">{label}</span><span className="block break-all font-mono">{value}</span></span>
        <button type="button" className="min-h-9 shrink-0 rounded-md border border-border px-3 text-sm font-medium hover:bg-surface" onClick={() => void copy()}>{t(copied ? 'setup.v2.copied' : 'setup.v2.copy')}</button>
    </div>;
}

// How the owner reaches the panel before its own name resolves: the server's
// address with the temporary certificate, said plainly.
export function SetupPanelAccessCard({ ip, port, panelDomain }: { ip: string; port: string; panelDomain: string }) {
    const { t } = useSetupWizardI18n();
    if (!ip) return null;
    const current = `https://${ip}:${port}`;
    return <section aria-labelledby="setup-panel-access-title" className="mt-5 space-y-3 rounded-lg border border-border bg-surface p-4 sm:p-5" data-setup-panel-access="">
        <h3 id="setup-panel-access-title" className="font-semibold">{t('setup.v2.access.title')}</h3>
        <SetupCopyRow label={t('setup.v2.access.current')} value={current} />
        <p className="max-w-3xl break-words text-sm leading-6 text-fg-muted">{t('setup.v2.access.help', { name: panelDomain || '—', secure: `https://${panelDomain || '—'}:${port}` })}</p>
    </section>;
}

// The registrar card on today's DNS backend (alpha.111): the exact nameserver
// names and, for names inside the main domain, their IP records (glue). The
// panel does not read the delegation itself in this release; setup notices it
// when the panel's own name starts to resolve.
export function SetupRegistrarCard({ draft, mainDomain, now }: { draft: ServerSetupDraft; mainDomain: string; now: boolean }) {
    const { t } = useSetupWizardI18n();
    const primary = draft.dns_role === 'primary';
    const rows = [
        { name: draft.ns1, ip: primary ? draft.local_ip : draft.peer_ip },
        { name: draft.ns2, ip: primary ? draft.peer_ip : draft.local_ip },
    ];
    const domain = mainDomain || draft.infrastructure_dns?.zone || '';
    const glue = rows.filter(row => domain && setupNameWithin(row.name, domain));
    return <section aria-labelledby="setup-registrar-title" className="mt-5 space-y-4" data-setup-registrar={now ? 'now' : 'later'}>
        <SetupLead title={t(now ? 'setup.v2.registrar.nowTitle' : 'setup.v2.registrar.laterTitle')}
            why={t(now ? 'setup.v2.registrar.nowWhy' : 'setup.v2.registrar.laterWhy', { domain: domain || '—', name: draft.panel_domain || '—' })}
            who={t('setup.v2.registrar.who')} next={t(glue.length ? 'setup.v2.registrar.nextGlue' : 'setup.v2.registrar.next')} then={t('setup.v2.registrar.then')} />
        <div className="space-y-4 rounded-lg border border-border bg-surface p-4 sm:p-5">
            <h3 id="setup-registrar-title" className="font-semibold">{t('setup.v2.registrar.title')}</h3>
            {glue.length > 0 ? <div className="space-y-2">
                <h4 className="text-sm font-semibold">{t('setup.v2.registrar.glueTitle')}</h4>
                <p className="text-sm leading-6 text-fg-muted">{t('setup.v2.registrar.glueHelp', { domain })}</p>
                {glue.map(row => <div key={row.name} className="grid gap-2 sm:grid-cols-2"><SetupCopyRow label={t('setup.v2.registrar.name')} value={row.name} /><SetupCopyRow label="IPv4" value={row.ip || '—'} /></div>)}
            </div> : <p className="text-sm leading-6 text-fg-muted">{t('setup.v2.registrar.noGlue')}</p>}
            <div className="space-y-2">
                <h4 className="text-sm font-semibold">{t(glue.length ? 'setup.v2.registrar.nsTitle' : 'setup.v2.registrar.nsTitleOnly')}</h4>
                <p className="text-sm leading-6 text-fg-muted">{t('setup.v2.registrar.nsHelp')}</p>
                <div className="grid gap-2 sm:grid-cols-2"><SetupCopyRow label={t('setup.v2.registrar.ns1')} value={draft.ns1} /><SetupCopyRow label={t('setup.v2.registrar.ns2')} value={draft.ns2} /></div>
            </div>
            <p className="text-sm leading-6 text-fg-muted">{t('setup.v2.registrar.spread')}</p>
        </div>
    </section>;
}

export interface SetupProviderRecords {
    mode: string;
    servers: { label: string; ip4?: string; rows: { name: string; type: string; value: string; state: 'ok' | 'missing' | 'different' | 'unknown' | 'kept' | 'pending'; seen: string[]; kind?: string }[] }[];
    checked_at: string;
}
export function decodeSetupProviderRecords(value: unknown): SetupProviderRecords {
    const record = (item: unknown): item is Record<string, unknown> => !!item && typeof item === 'object' && !Array.isArray(item);
    const invalid = () => new Error('provider records outside the contract');
    if (!record(value) || typeof value.mode !== 'string' || typeof value.checked_at !== 'string' || !Array.isArray(value.servers) || value.servers.length > 4) throw invalid();
    for (const server of value.servers) {
        if (!record(server) || typeof server.label !== 'string' || !Array.isArray(server.rows) || server.rows.length > 8) throw invalid();
        for (const row of server.rows) {
            if (!record(row) || typeof row.name !== 'string' || typeof row.type !== 'string' || typeof row.value !== 'string'
                || !['ok', 'missing', 'different', 'unknown', 'kept', 'pending'].includes(String(row.state)) || !Array.isArray(row.seen) || row.seen.some(item => typeof item !== 'string')) throw invalid();
        }
    }
    return value as unknown as SetupProviderRecords;
}

const providerState: Record<string, TranslationKey> = { ok: 'setup.v2.provider.ok', missing: 'setup.v2.provider.missing', different: 'setup.v2.provider.different', unknown: 'setup.v2.provider.unknown', kept: 'setup.v3.provider.kept', pending: 'setup.v3.provider.pending' };

// The records to add at an external DNS provider, each with what the internet
// answers now. The page only reads; the server shares one answer per minute.
export function SetupProviderRecordsCard() {
    const { t } = useSetupWizardI18n();
    // Read through the shared layer: loading, known and unknown stay apart;
    // the poll only reads (the server answers from its one-minute share).
    const remote = useRemote('/api/v1/setup/provider-records', decodeSetupProviderRecords);
    useRefreshEvery(remote, 60_000);
    const value = remote.remote.state === 'known' ? remote.remote.value : remote.remote.state === 'unknown' ? remote.remote.previous?.value ?? null : null;
    const failed = remote.remote.state === 'unknown';
    const rows = value?.servers.flatMap(server => server.rows) || [];
    const allOK = rows.length > 0 && rows.every(row => row.state === 'ok' || row.state === 'kept');
    return <section aria-labelledby="setup-provider-title" className="mt-5 space-y-4" data-setup-provider-records="">
        <SetupLead tone={allOK ? 'done' : 'neutral'} title={t(allOK ? 'setup.v2.provider.doneTitle' : 'setup.v2.provider.leadTitle')} why={t('setup.v2.provider.why')} who={t(allOK ? 'setup.v2.provider.whoNone' : 'setup.v2.provider.who')} next={t(allOK ? 'setup.v2.provider.nextNone' : 'setup.v2.provider.next')} then={t('setup.v2.provider.then')} />
        <div className="space-y-3 rounded-lg border border-border bg-surface p-4 sm:p-5">
            <h3 id="setup-provider-title" className="font-semibold">{t('setup.v2.provider.title')}</h3>
            {!value ? <p role="status" className="text-sm text-fg-muted">{t(failed ? 'setup.v2.provider.failed' : 'setup.v2.provider.loading')}</p>
                : rows.length === 0 ? <p className="text-sm text-fg-muted">{t('setup.v2.provider.none')}</p>
                    : <ul className="space-y-3">{rows.map(row => <li key={`${row.name}-${row.type}`} className="space-y-2" data-record={row.name} data-state={row.state}>
                        <div className="grid gap-2 sm:grid-cols-2"><SetupCopyRow label={`${t('setup.v2.registrar.name')} · ${row.type}`} value={row.name} />{row.value ? <SetupCopyRow label={row.type === 'A' ? 'IPv4' : row.type === 'AAAA' ? 'IPv6' : t('setup.v3.provider.value')} value={row.value} /> : null}</div>
                        <p className={`text-sm ${row.state === 'ok' ? 'text-success' : row.state === 'different' ? 'text-danger' : 'text-fg-muted'}`}>{t(providerState[row.state], { seen: row.seen.join(', ') || '—' })}</p>
                    </li>)}</ul>}
            {failed && value && <p role="status" className="text-sm text-fg-muted">{t('setup.v2.provider.failed')}</p>}
            <p className="text-sm leading-6 text-fg-muted">{t('setup.v2.provider.cloudflare')}</p>
            {value && <p className="text-xs text-fg-muted">{t('setup.v2.provider.checkedAt', { time: new Date(value.checked_at).toLocaleTimeString() })}</p>}
        </div>
    </section>;
}

// ---- Phase 2 (alpha.112): existing records and the main domain's mail ----

export interface SetupPublicRecords {
    domain: string;
    extras: string[];
    records: { name: string; type: string; values: string[]; state: 'found' | 'none' | 'unknown' }[];
    checked_at: string;
}
export function decodeSetupPublicRecords(value: unknown): SetupPublicRecords {
    const record = (item: unknown): item is Record<string, unknown> => !!item && typeof item === 'object' && !Array.isArray(item);
    const invalid = () => new Error('public records outside the contract');
    if (!record(value) || typeof value.domain !== 'string' || !Array.isArray(value.extras) || !Array.isArray(value.records) || value.records.length > 128) throw invalid();
    for (const item of value.records) {
        if (!record(item) || typeof item.name !== 'string' || typeof item.type !== 'string' || !Array.isArray(item.values)
            || item.values.some(entry => typeof entry !== 'string') || !['found', 'none', 'unknown'].includes(String(item.state))) throw invalid();
    }
    return value as unknown as SetupPublicRecords;
}

type ImportType = SetupImportedRecord['type'];
const importTypes: ImportType[] = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'CAA'];
interface ImportCandidate { record: SetupImportedRecord; clash: boolean }

// One candidate per public value. A name this setup gives to a server cannot
// keep its old address record: it is skipped and the screen says why.
function importCandidates(read: SetupPublicRecords, draft: ServerSetupDraft, own: string[]): ImportCandidate[] {
    const out: ImportCandidate[] = [];
    for (const item of read.records) {
        if (item.state !== 'found' || !importTypes.includes(item.type as ImportType)) continue;
        for (const value of item.values) {
            let content = value, priority: number | undefined;
            if (item.type === 'MX') { const [pref, host] = value.split(/\s+/); priority = Number(pref) || 0; content = host || ''; }
            const clash = own.includes(item.name) && ['A', 'AAAA', 'CNAME'].includes(item.type);
            const saved = draft.imported_records?.find(entry => entry.name === item.name && entry.type === item.type && entry.content === content);
            out.push({ clash, record: { name: item.name, type: item.type as ImportType, content, ...(priority !== undefined ? { priority } : {}), decision: clash ? 'skip' : saved?.decision || 'import' } });
        }
    }
    return out;
}

export function SetupRecordsStep({ draft, update, mainDomain, own }: {
    draft: ServerSetupDraft;
    update: (change: (draft: ServerSetupDraft) => ServerSetupDraft) => void;
    mainDomain: string;
    own: string[];
}) {
    const { t } = useSetupWizardI18n();
    const [extras, setExtras] = useState<string[]>(() => [...new Set((draft.imported_records || []).map(item => item.name.slice(0, -(mainDomain.length + 1))).filter(label => label && !['www', 'mail', 'webmail', 'autodiscover', '_dmarc'].includes(label)))].slice(0, 8));
    const [typed, setTyped] = useState('');
    const url = `/api/v1/setup/public-records${extras.length ? `?extra=${encodeURIComponent(extras.join(','))}` : ''}`;
    const remote = useRemote(url, decodeSetupPublicRecords);
    const read = remote.remote.state === 'known' ? remote.remote.value : null;
    const candidates = read ? importCandidates(read, draft, own) : [];
    const wanted = JSON.stringify(candidates.map(item => item.record));
    useEffect(() => {
        if (!read) return;
        update(previous => JSON.stringify(previous.imported_records || []) === wanted ? previous : { ...previous, imported_records: JSON.parse(wanted) });
        // `wanted` is the whole decision list; the read itself only changes it.
    }, [wanted]);
    const decide = (index: number, decision: 'import' | 'skip') => update(previous => {
        const list = candidates.map(item => item.record);
        list[index] = { ...list[index], decision };
        return { ...previous, imported_records: list };
    });
    const unread = read?.records.filter(item => item.state === 'unknown') || [];
    const imported = candidates.filter(item => item.record.decision === 'import').length;
    const mailClash = candidates.some(item => item.clash && item.record.name === draft.mail_hostname);
    return <section className="space-y-5" aria-labelledby="setup-records-title" data-setup-records="">
        <h2 id="setup-records-title" className="text-xl font-semibold">{t('setup.v3.records.title')}</h2>
        <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v3.records.help', { domain: mainDomain })}</p>
        {remote.remote.state === 'loading' && <p role="status" className="text-sm text-fg-muted">{t('setup.v3.records.reading')}</p>}
        {remote.remote.state === 'unknown' && <div role="status" className="space-y-2 rounded-lg border border-border bg-surface p-4 text-sm"><p>{t('setup.v3.records.failed')}</p><Button type="button" variant="secondary" onClick={() => void remote.retry()}>{t('common.retry')}</Button></div>}
        {read && (candidates.length === 0
            ? <p className="text-sm leading-6 text-fg-muted">{t('setup.v3.records.none', { domain: mainDomain })}</p>
            : <ul className="divide-y divide-border rounded-lg border border-border bg-surface">{candidates.map((item, index) => {
                const record = item.record;
                return <li key={`${record.name}-${record.type}-${record.content}`} className="space-y-2 px-4 py-3 text-sm" data-import={`${record.name} ${record.type}`} data-decision={record.decision}>
                    <div className="flex flex-wrap items-baseline justify-between gap-2">
                        <span className="min-w-0 break-all font-mono">{record.name} <span className="text-fg-muted">{record.type}</span> {record.priority !== undefined ? `${record.priority} ` : ''}{record.content}</span>
                        {item.clash ? <span className="text-fg-muted">{t('setup.v3.records.clash')}</span>
                            : <span role="radiogroup" aria-label={record.name} className="flex gap-3">
                                {(['import', 'skip'] as const).map(choice => <label key={choice} className="inline-flex cursor-pointer items-center gap-1.5"><input type="radio" name={`setup-import-${index}`} value={choice} checked={record.decision === choice} onChange={() => decide(index, choice)} className="h-4 w-4 accent-primary" />{t(choice === 'import' ? 'setup.v3.records.import' : 'setup.v3.records.skip')}</label>)}
                            </span>}
                    </div>
                    {item.clash && <p className="leading-6 text-fg-muted">{t('setup.v3.records.clashHelp', { name: record.name })}</p>}
                    {!item.clash && record.decision === 'skip' && <p className="leading-6 text-fg-muted">{t('setup.v3.records.skipHelp')}</p>}
                </li>;
            })}</ul>)}
        {mailClash && <div className="space-y-2 rounded-lg border border-border bg-surface p-4 text-sm leading-6">
            <p>{t('setup.v3.records.mailClash', { name: draft.mail_hostname })}</p>
            <Button type="button" variant="secondary" onClick={() => update(previous => ({ ...previous, mail_hostname: `mail2.${mainDomain}` }))}>{t('setup.v3.records.useMail2', { name: `mail2.${mainDomain}` })}</Button>
        </div>}
        {unread.length > 0 && <p role="status" className="text-sm leading-6 text-fg-muted">{t('setup.v3.records.unread', { names: [...new Set(unread.map(item => item.name))].join(', ') })}</p>}
        {read && <p className="text-sm text-fg-muted">{t('setup.v3.records.count', { imported, skipped: candidates.length - imported })}</p>}
        <div className="flex max-w-md flex-wrap items-end gap-2">
            <div className="min-w-0 flex-1"><SetupWizardInput name="extra_name" label={t('setup.v3.records.extra')} value={typed} onChange={setTyped} placeholder={`shop.${mainDomain || 'example.com'}`} /></div>
            <Button type="button" variant="secondary" disabled={!typed.trim() || extras.length >= 8} onClick={() => { const label = typed.trim().toLowerCase().replace(/\.$/, '').replace(new RegExp(`\\.${mainDomain.replace(/\./g, '\\.')}$`), ''); if (label && !extras.includes(label)) setExtras([...extras, label]); setTyped(''); }}>{t('setup.v3.records.look')}</Button>
        </div>
        <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v3.records.limit')}</p>
    </section>;
}

export function SetupMailStep({ draft, update, mainDomain, ip4, ip6 }: {
    draft: ServerSetupDraft;
    update: (change: (draft: ServerSetupDraft) => ServerSetupDraft) => void;
    mainDomain: string;
    ip4: string;
    ip6: string;
}) {
    const { t } = useSetupWizardI18n();
    const host = draft.mail_hostname || `mail.${mainDomain}`;
    const local = draft.dns_mode === 'local' && draft.dns_role === 'primary';
    const oldMX = setupImportedMX(draft, mainDomain);
    const oldDMARC = setupImportedDMARC(draft, mainDomain);
    const askMX = oldMX.length > 0 || draft.dns_mode === 'external';
    const mx = draft.mail_dns?.mx || (askMX ? 'later' : 'now');
    const dmarc = draft.mail_dns?.dmarc || (oldDMARC ? 'keep' : 'add');
    const choose = (key: 'mx' | 'dmarc', value: string) => update(previous => ({ ...previous, mail_dns: { ...previous.mail_dns, [key]: value } }));
    const spf = mergeSetupSPF(setupImportedSPF(draft, mainDomain), host);
    return <section className="space-y-5" aria-labelledby="setup-mail-title" data-setup-mail="">
        <h2 id="setup-mail-title" className="text-xl font-semibold">{t('setup.v3.mail.title')}</h2>
        <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t(local ? 'setup.v3.mail.helpLocal' : draft.dns_mode === 'external' ? 'setup.v3.mail.helpExternal' : 'setup.v3.mail.helpElsewhere', { domain: mainDomain })}</p>
        <ul className="space-y-2 text-sm">
            <li className="rounded-md border border-border bg-surface-2 px-3 py-2"><span className="font-medium">MX</span> · <span className="break-all font-mono">{mx === 'now' ? `10 ${host}` : t('setup.v3.mail.mxKept', { old: oldMX.map(item => item.content).join(', ') || '—' })}</span></li>
            <li className="rounded-md border border-border bg-surface-2 px-3 py-2"><span className="font-medium">SPF</span> · <span className="break-all font-mono">{spf}</span>{setupImportedSPF(draft, mainDomain) && <span className="block text-xs text-fg-muted">{t('setup.v3.mail.spfMerged')}</span>}</li>
            <li className="rounded-md border border-border bg-surface-2 px-3 py-2"><span className="font-medium">DKIM</span> · {t('setup.v3.mail.dkim')}</li>
            <li className="rounded-md border border-border bg-surface-2 px-3 py-2"><span className="font-medium">DMARC</span> · <span className="break-all font-mono">{dmarc === 'keep' && oldDMARC ? oldDMARC.content : `v=DMARC1; p=none; rua=mailto:dmarc@${mainDomain}`}</span></li>
        </ul>
        {askMX && <fieldset className="space-y-2"><legend className="font-semibold">{t('setup.v3.mail.mxTitle')}</legend>
            <div className="grid gap-3 lg:grid-cols-2">
                <Choice name="setup-mail_mx" value="later" checked={mx === 'later'} onChange={() => choose('mx', 'later')} title={t('setup.v3.mail.mxLater')} tag={t('setup.v2.recommended')} help={t('setup.v3.mail.mxLaterHelp')} />
                <Choice name="setup-mail_mx" value="now" checked={mx === 'now'} onChange={() => choose('mx', 'now')} title={t('setup.v3.mail.mxNow')} help={t('setup.v3.mail.mxNowHelp', { host })} />
            </div>
        </fieldset>}
        {oldDMARC && <fieldset className="space-y-2"><legend className="font-semibold">DMARC</legend>
            <div className="grid gap-3 lg:grid-cols-2">
                <Choice name="setup-mail_dmarc" value="keep" checked={dmarc === 'keep'} onChange={() => choose('dmarc', 'keep')} title={t('setup.v3.mail.dmarcKeep')} help={t('setup.v3.mail.dmarcKeepHelp')} />
                <Choice name="setup-mail_dmarc" value="add" checked={dmarc === 'add'} onChange={() => choose('dmarc', 'add')} title={t('setup.v3.mail.dmarcAdd')} help={t('setup.v3.mail.dmarcAddHelp')} />
            </div>
        </fieldset>}
        <div className="space-y-2 rounded-lg border border-border bg-surface p-4 text-sm leading-6" data-setup-ptr="">
            <h3 className="font-semibold">{t('setup.v3.mail.ptrTitle')}</h3>
            <p className="text-fg-muted">{t('setup.v3.mail.ptrHelp', { host })}</p>
            {ip4 && <div className="grid gap-2 sm:grid-cols-2"><SetupCopyRow label="IPv4" value={ip4} /><SetupCopyRow label="PTR" value={host} /></div>}
            {ip6 && <div className="grid gap-2 sm:grid-cols-2"><SetupCopyRow label="IPv6" value={ip6} /><SetupCopyRow label="PTR" value={host} /></div>}
            <p className="text-fg-muted">{t('setup.v3.mail.port25')}</p>
        </div>
    </section>;
}
