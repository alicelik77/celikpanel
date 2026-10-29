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
        {value === 2 && <p className="max-w-2xl text-sm leading-6 text-fg-muted" role="note">{t('setup.v5.dns.second.help')}</p>}
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
        { role: 'dns', title: 'setup.v2.role.dns', help: serverCount === 2 ? 'setup.v2.role.dns.helpTwo' : 'setup.v5.dns.standalone.help', brings: t('setup.v2.role.dnsInstalls') },
    ];
    return <fieldset className="space-y-4" disabled={disabled}>
        <legend className="text-xl font-semibold">{t('setup.v2.roles.title', { server })}</legend>
        <p className="max-w-2xl pt-2 text-sm leading-6 text-fg-muted">{t('setup.v2.roles.help')}</p>
        <div className="grid gap-3">{cards.map(card => <Choice key={card.role} type="checkbox" name="setup-role" value={card.role} checked={roles[card.role]} disabled={!catalog && card.role !== 'dns'}
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
    server_setup_pdns_main_untested_version: 'setup.v2.engine.untestedVersion',
    server_setup_pdns_main_not_debian_archive: 'setup.v2.engine.notDebianArchive',
    server_setup_existing_dns_requires_migration: 'setup.v2.engine.existing',
    bind_source_pdns_switch_unsupported: 'setup.v2.engine.existing',
    pdns_primary_switch_paused: 'setup.v2.engine.paused',
    server_setup_dns_engine_unsupported: 'setup.v2.engine.unsupported',
    server_setup_inventory_unavailable: 'setup.v2.engine.unknown',
};

export function setupEngineChoice(catalog: SetupComponentCatalog | null, engine: 'bind' | 'pdns', role: 'primary' | 'secondary'): SetupDNSEngineChoice | undefined {
    return catalog?.dns_engines?.find(item => item.id === engine && item.role === (role === 'primary' ? 'main' : 'backup'));
}

export function SetupDNSHomeStep({ draft, set, onMode, onMainDNS, catalog, serverCount, mainDomain, detectedIP, connection, publisher, joined }: {
    draft: ServerSetupDraft;
    set: <K extends keyof ServerSetupDraft>(key: K, value: ServerSetupDraft[K]) => void;
    onMode: (mode: ServerSetupDraft['dns_mode']) => void;
    onMainDNS: (main: 'this' | 'peer') => void;
    catalog: SetupComponentCatalog | null;
    serverCount: 1 | 2;
    mainDomain: string;
    detectedIP: string;
    connection: ReactNode;
    publisher: ReactNode;
    joined?: SetupPeerLink | null;
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
    const oneServer = serverCount === 1 && !joined;
    // ns1 is the main DNS server, ns2 the backup (D-021 Q2): the label says
    // which slot a name fills, whichever server this is.
    const slotLabel = (key: 'ns1' | 'ns2') => t(key === 'ns1' ? 'setup.v5.dns.mainName' : 'setup.v5.dns.backupName');
    const software = <fieldset className="space-y-3"><legend className="font-semibold">{t('setup.v2.dns.software')}</legend>
        <div className="grid gap-3 lg:grid-cols-2">{(['bind', 'pdns'] as const).map(id => {
            const choice = engine(id);
            const blocked = !!choice && !choice.offered;
            return <Choice key={id} name="setup-dns_engine" value={id} checked={draft.dns_engine === id} disabled={blocked && draft.dns_engine !== id} onChange={() => set('dns_engine', id)}
                title={<>{id === 'bind' ? 'BIND' : 'PowerDNS'}{version(id) && <span className="font-normal text-fg-muted">{version(id)}</span>}</>} tag={id === 'bind' ? t('setup.v2.recommended') : undefined} help={engineHelp(id)} />;
        })}</div>
        {engine(draft.dns_engine) && !engine(draft.dns_engine)!.offered && <p role="alert" className="text-sm leading-6 text-danger">{engineHelp(draft.dns_engine)}</p>}
        <p className="text-sm leading-6 text-fg-muted">{t('setup.v2.dns.oneSoftware')}</p>
    </fieldset>;
    const localIP = <SetupWizardInput name="local_ip" label={t('setup.v2.dns.ipv4')} value={draft.local_ip} onChange={value => set('local_ip', value)} required help={detectedIP && draft.local_ip === detectedIP ? t('setup.v2.dns.detected') : undefined} />;
    return <section className="space-y-6" aria-labelledby="setup-dnshome-title">
        <div><h2 id="setup-dnshome-title" className="text-xl font-semibold">{t('setup.v2.dns.title')}</h2>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v2.dns.help', { domain: mainDomain || 'example.com' })}</p></div>
        {joined ? <div className="space-y-6" data-setup-dns-joined="">
            <p className="max-w-2xl text-sm leading-6">{t('setup.v5.dns.joined.help', { role: t(primary ? 'setup.v5.dns.joined.roleMain' : 'setup.v5.dns.joined.roleBackup'), domain: joined.main_domain })}</p>
            <dl className="grid max-w-2xl gap-2 rounded-lg border border-border bg-surface p-4 text-sm sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]">
                <dt className="text-fg-muted">{slotLabel('ns1')}</dt><dd className="break-all font-mono">{joined.ns1}</dd>
                <dt className="text-fg-muted">{slotLabel('ns2')}</dt><dd className="break-all font-mono">{joined.ns2}</dd>
            </dl>
            {software}
        </div> : <>
        <div className="grid gap-3">
            <Choice name="setup-dns" value="local" checked={draft.dns_mode === 'local'} onChange={() => onMode('local')} title={t('setup.v2.dns.local')} tag={oneServer ? undefined : t('setup.v2.recommended')} help={t(oneServer ? 'setup.v5.dns.standalone.help' : 'setup.v2.dns.local.help')} />
            <Choice name="setup-dns" value="external" checked={draft.dns_mode === 'external'} onChange={() => onMode('external')} title={t('setup.v2.dns.external')} help={t('setup.v2.dns.external.help')} />
            <Choice name="setup-dns" value="existing" checked={draft.dns_mode === 'existing'} onChange={() => onMode('existing')} title={t('setup.v2.dns.existing')} help={t('setup.v2.dns.existing.help')} />
        </div>
        {draft.dns_mode === 'external' && <div className="rounded-lg border border-border bg-surface p-4 text-sm leading-6"><p className="font-semibold">{t('setup.v2.dns.withChoice')}</p>
            <ul className="mt-2 list-disc space-y-1 pl-5 text-fg-muted"><li>{t('setup.v2.dns.external.point1')}</li><li>{t('setup.v2.dns.external.point2')}</li><li>{t('setup.v2.dns.external.point3')}</li></ul></div>}
        {draft.dns_mode === 'existing' && connection}
        {draft.dns_mode === 'local' && oneServer && <div className="space-y-6" data-setup-dns-standalone="">
            <fieldset className="min-w-0 space-y-4 rounded-xl border border-border bg-surface p-4 sm:p-5">
                <legend className="font-semibold">{t('setup.v5.dns.standalone.title')}</legend>
                <p className="text-sm leading-6 text-fg-muted">{t('setup.v5.dns.standalone.help')}</p>
                <div className="grid gap-4 lg:grid-cols-2">
                    <SetupWizardInput name="ns1" label={slotLabel('ns1')} value={draft.ns1} onChange={value => set('ns1', value)} placeholder={`ns1.${mainDomain || 'example.com'}`} required />
                    <SetupWizardInput name="ns2" label={slotLabel('ns2')} value={draft.ns2} onChange={value => set('ns2', value)} placeholder={`ns2.${mainDomain || 'example.com'}`} required />
                </div>
                {localIP}
            </fieldset>
            {software}
        </div>}
        {draft.dns_mode === 'local' && !oneServer && <div className="space-y-6">
            <fieldset className="space-y-3"><legend className="font-semibold">{t('setup.v5.dns.main.title')}</legend>
                <div className="grid gap-3 lg:grid-cols-2">
                    <Choice name="setup-main_dns_server" value="this" checked={primary} onChange={() => onMainDNS('this')} title={t('setup.v5.dns.main.this')} help={t('setup.v5.dns.main.thisHelp')} />
                    <Choice name="setup-main_dns_server" value="peer" checked={!primary} onChange={() => onMainDNS('peer')} title={t('setup.v5.dns.main.second')} help={t('setup.v5.dns.main.secondHelp')} />
                </div>
            </fieldset>
            <div className="grid gap-6 rounded-xl border border-border bg-surface p-4 sm:p-5 lg:grid-cols-2 lg:gap-8">
                <fieldset className="min-w-0 space-y-4">
                    <legend className="mb-0 font-semibold">{t(primary ? 'setup.v2.dns.thisMain' : 'setup.v2.dns.thisBackup')}</legend>
                    <SetupWizardInput name={names.localKey} label={slotLabel(names.localKey)} value={draft[names.localKey]} onChange={value => set(names.localKey, value)} placeholder={`${names.localKey}.${mainDomain || 'example.com'}`} required />
                    {localIP}
                </fieldset>
                <fieldset className="min-w-0 space-y-4 border-t border-border pt-5 lg:border-l lg:border-t-0 lg:pl-8 lg:pt-0" data-setup-second-server="">
                    <legend className="mb-0 font-semibold">{t('setup.v5.dns.second.title')}</legend>
                    <SetupWizardInput name={names.peerKey} label={slotLabel(names.peerKey)} value={draft[names.peerKey]} onChange={value => { set(names.peerKey, value); set('peer_ns', value); }} placeholder={`${names.peerKey}.${mainDomain || 'example.com'}`} required />
                    <p className="text-sm leading-6 text-fg-muted">{t('setup.v5.dns.second.help')}</p>
                </fieldset>
            </div>
            {software}
            {publisher}
        </div>}
        </>}
    </section>;
}

// --- The second server joining by code (alpha.114) -------------------------

export interface SetupPeerLink {
    address: string;
    main_domain: string;
    main_dns_server: 'first' | 'second';
    ns1: string;
    ns2: string;
    first: { ipv4: string; short_name?: string; panel_domain: string; mail_hostname?: string; main_domain_mail?: boolean; dns_engine: string };
    imported: number;
    joined_at: string;
}
export interface SetupPeerView {
    role: 'none' | 'first' | 'second';
    code?: { address: string; expires_at: string; attempts_left: number; used: boolean };
    joined?: { short_name?: string; ipv4: string; joined_at: string; panel_domain?: string; mail_hostname?: string; main_domain_mail?: boolean };
    link?: SetupPeerLink;
    pending?: boolean;
}
const peerRecord = (item: unknown): item is Record<string, unknown> => !!item && typeof item === 'object' && !Array.isArray(item);
const peerText = (value: unknown, limit = 253) => typeof value === 'string' && value.length <= limit;
export function decodeSetupPeerView(value: unknown): SetupPeerView {
    const invalid = () => new Error('invalid peer state');
    if (!peerRecord(value) || !['none', 'first', 'second'].includes(String(value.role))) throw invalid();
    const code = value.code, joined = value.joined, link = value.link;
    if (code !== undefined && (!peerRecord(code) || !peerText(code.address) || !peerText(code.expires_at, 64) || typeof code.attempts_left !== 'number' || typeof code.used !== 'boolean')) throw invalid();
    if (joined !== undefined && (!peerRecord(joined) || !peerText(joined.ipv4, 64) || !peerText(joined.joined_at, 64))) throw invalid();
    if (link !== undefined && (!peerRecord(link) || !peerText(link.address) || !peerText(link.main_domain) || !['first', 'second'].includes(String(link.main_dns_server)) || !peerText(link.ns1) || !peerText(link.ns2)
        || !peerRecord(link.first) || !peerText(link.first.ipv4, 64) || !peerText(link.first.panel_domain) || typeof link.imported !== 'number')) throw invalid();
    return value as unknown as SetupPeerView;
}

// A public IPv4 address as the wizard can tell: not private, loopback,
// link-local, shared or multicast. / Genel IPv4 adresi.
function publicSetupIPv4(value: string): boolean {
    const parts = value.split('.').map(Number);
    if (parts.length !== 4 || parts.some(part => !Number.isInteger(part) || part < 0 || part > 255)) return false;
    const [a, b] = parts;
    return !(a === 0 || a === 10 || a === 127 || a >= 224 || (a === 169 && b === 254) || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) || (a === 100 && b >= 64 && b <= 127));
}

type SetupConnectAnswer = { state: 'ok' | 'refused' | 'wrong_server' | 'unreachable' | 'unknown'; code?: string; attempts_left?: number; setup?: unknown };

// On the second server, before its setup starts: the first server's address
// and code. The answer's state leads (D-024): refused names its reason; a
// wrong server, an unreachable one and a lost answer say that nothing was
// sent or that the same join is asked again.
export function SetupJoinPanel({ onJoined, disabled, serverIP }: { onJoined: (setup: unknown) => void; disabled?: boolean; serverIP?: string }) {
    const { t } = useSetupWizardI18n();
    const [open, setOpen] = useState(false);
    const [address, setAddress] = useState('');
    const [code, setCode] = useState('');
    const [publicIPv4, setPublicIPv4] = useState('');
    const [busy, setBusy] = useState(false);
    const [answer, setAnswer] = useState<SetupConnectAnswer | null>(null);
    const [problem, setProblem] = useState<TranslationKey | null>(null);
    const inFlight = useRef(false);
    async function join() {
        if (inFlight.current) return;
        if (!address.trim() || code.replace(/[\s-]/g, '').length !== 24) { setProblem('setup.v5.join.invalid'); return; }
        inFlight.current = true; setBusy(true); setProblem(null); setAnswer(null);
        try {
            const response = await fetch('/api/v1/setup/peer/connect', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ address: address.trim(), code: code.trim(), ...(publicIPv4.trim() ? { public_ipv4: publicIPv4.trim() } : {}) }) });
            const body: unknown = await response.json().then((value: unknown) => value, (cause: unknown) => ({ unreadable: String(cause) }));
            if (!response.ok) {
                const coded = peerRecord(body) && typeof body.code === 'string' ? body.code : '';
                setProblem(coded === 'server_setup_peer_connect_invalid' ? 'setup.v5.join.invalid' : coded === 'server_setup_peer_connect_running' ? 'setup.v5.join.running' : coded === 'server_setup_peer_connect_public_ipv4' ? 'setup.v5.join.publicInvalid' : coded === 'server_setup_peer_connect_public_ipv4_required' ? 'setup.v5.join.publicRequired' : 'setup.v5.join.failed');
                return;
            }
            if (!peerRecord(body) || !['ok', 'refused', 'wrong_server', 'unreachable', 'unknown'].includes(String(body.state))) { setProblem('setup.v5.join.failed'); return; }
            const value = body as unknown as SetupConnectAnswer;
            setAnswer(value);
            if (value.state === 'ok' && value.setup) onJoined(value.setup);
        } catch {
            // The request may not have left this page: nothing is assumed.
            setProblem('setup.v5.join.failed');
        } finally { inFlight.current = false; setBusy(false); }
    }
    const refused = (value: SetupConnectAnswer) => {
        const known = ['server_setup_join_code_wrong', 'server_setup_join_code_used', 'server_setup_join_code_expired', 'server_setup_join_code_exhausted', 'server_setup_join_code_unavailable', 'server_setup_join_source_mismatch', 'server_setup_join_not_waiting', 'server_setup_join_rate_limited', 'server_setup_join_same_server', 'server_setup_join_address_not_public', 'server_setup_join_certificate_unavailable', 'server_setup_peer_version_incompatible'];
        return known.includes(value.code || '') ? t(`setup.v5.join.refused.${value.code}` as TranslationKey, { left: value.attempts_left ?? 0 }) : t('setup.v5.join.refused.other', { code: value.code || '' });
    };
    const message = answer?.state === 'ok' ? answer.setup ? '' : t('setup.v5.join.draftRetry')
        : answer ? answer.state === 'refused' ? refused(answer)
        : t(answer.state === 'wrong_server' ? 'setup.v5.join.state.wrongServer' : answer.state === 'unreachable' ? 'setup.v5.join.state.unreachable' : 'setup.v5.join.state.unknown') : '';
    return <section aria-labelledby="setup-join-title" className="mt-8 space-y-3 border-t border-border pt-6" data-setup-join="">
        <h3 id="setup-join-title" className="font-semibold">{t('setup.v5.join.title')}</h3>
        <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v5.join.help')}</p>
        {!open ? <Button type="button" variant="secondary" disabled={disabled} onClick={() => setOpen(true)}>{t('setup.v5.join.open')}</Button> : <div className="max-w-xl space-y-4">
            <SetupWizardInput name="join_address" label={t('setup.v5.join.address')} value={address} onChange={setAddress} placeholder="https://203.0.113.10:2083" help={t('setup.v5.join.addressHelp')} />
            <SetupWizardInput name="join_code" label={t('setup.v5.join.code')} value={code} onChange={setCode} placeholder="XXXX-XXXX-XXXX-XXXX-XXXX-XXXX" help={t('setup.v5.join.codeHelp')} />
            {/* The first server checks the caller's address against the one this
                server declares. Behind 1:1 NAT the own address is private: the
                owner types the public one (D-024: who acts and what to type). */}
            {serverIP && <p className="text-sm leading-6 text-fg-muted" data-setup-join-ip="">{t(publicSetupIPv4(serverIP) ? 'setup.v5.join.detected' : 'setup.v5.join.noPublic', { ip: serverIP })}</p>}
            {(!publicSetupIPv4(serverIP || '') || answer?.code === 'server_setup_join_source_mismatch' || problem === 'setup.v5.join.publicRequired' || publicIPv4) && <SetupWizardInput name="join_public_ipv4" label={t('setup.v5.join.publicIPv4')} value={publicIPv4} onChange={setPublicIPv4} placeholder="203.0.113.20" help={t('setup.v5.join.publicIPv4Help')} />}
            {problem && <p role="alert" className="text-sm leading-6 text-danger">{t(problem)}</p>}
            {message && <p role={answer?.state === 'refused' ? 'alert' : 'status'} className="text-sm leading-6" data-setup-join-state={answer?.state}>{message}</p>}
            <Button type="button" variant="primary" disabled={busy || disabled} loading={busy} onClick={() => void join()}>{t(busy ? 'setup.v5.join.joining' : answer && !(answer.state === 'ok' && answer.setup) ? 'setup.v5.join.retry' : 'setup.v5.join.submit')}</Button>
        </div>}
    </section>;
}

// The link state of this server (both sides): read once when the wizard
// opens and again after a join. / Bu sunucunun baglanti durumu.
export function useSetupPeerLink(enabled: boolean): { link: SetupPeerLink | null; known: boolean; refresh: () => void } {
    const remote = useRemote(enabled ? '/api/v1/setup/peer' : null, decodeSetupPeerView);
    const view = remote.remote.state === 'known' ? remote.remote.value : null;
    return { link: view?.role === 'second' && view.link ? view.link : null, known: remote.remote.state === 'known', refresh: () => { void remote.retry(); } };
}

// A joined second server's shared answers, fixed by the first server.
export function SetupJoinedSummary({ link }: { link: SetupPeerLink }) {
    const { t } = useSetupWizardI18n();
    const server = link.first.short_name || t('setup.v5.joined.firstServer');
    return <section className="space-y-4" aria-labelledby="setup-joined-title" data-setup-joined="">
        <h2 id="setup-joined-title" className="text-xl font-semibold">{t('setup.v5.joined.title')}</h2>
        <p className="max-w-2xl text-sm leading-6">{t('setup.v5.joined.help', { server, ip: link.first.ipv4, domain: link.main_domain, main: t(link.main_dns_server === 'first' ? 'setup.v5.joined.mainFirst' : 'setup.v5.joined.mainThis'), ns1: link.ns1, ns2: link.ns2 })}</p>
        {link.main_dns_server === 'second' && link.imported > 0 && <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t('setup.v5.joined.imported', { count: link.imported, domain: link.main_domain })}</p>}
    </section>;
}

// On the first server while its setup waits for the second: the code, made
// on request and shown once, and what the second server has done so far. The
// state is read about every ten seconds; reading never makes a code.
export function SetupJoinCard() {
    const { t } = useSetupWizardI18n();
    const remote = useRemote('/api/v1/setup/peer', decodeSetupPeerView);
    useRefreshEvery(remote, 10_000);
    const [made, setMade] = useState<{ code: string; address: string; expires_at: string; attempts_left: number } | null>(null);
    const [busy, setBusy] = useState(false);
    const [problem, setProblem] = useState('');
    const inFlight = useRef(false);
    const view = remote.remote.state === 'known' ? remote.remote.value : remote.remote.state === 'unknown' ? remote.remote.previous?.value ?? null : null;
    async function create() {
        if (inFlight.current) return;
        inFlight.current = true; setBusy(true); setProblem('');
        try {
            const response = await fetch('/api/v1/setup/join-codes', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
            const body: unknown = await response.json().then((value: unknown) => value, (cause: unknown) => ({ unreadable: String(cause) }));
            if (response.ok && peerRecord(body) && typeof body.code === 'string' && typeof body.address === 'string' && typeof body.expires_at === 'string' && typeof body.attempts_left === 'number') {
                setMade({ code: body.code, address: body.address, expires_at: body.expires_at, attempts_left: body.attempts_left });
            } else {
                const coded = peerRecord(body) && typeof body.code === 'string' ? body.code : '';
                setProblem(['server_setup_join_not_waiting', 'server_setup_join_bound', 'server_setup_join_reopen_required', 'server_setup_join_finished', 'server_setup_join_certificate_unavailable'].includes(coded) ? t(`setup.v5.card.error.${coded}` as TranslationKey) : t('setup.v5.card.error.other'));
            }
        } catch {
            setProblem(t('setup.v5.card.error.other'));
        } finally { inFlight.current = false; setBusy(false); void remote.retry(); }
    }
    const time = (value: string) => new Date(value).toLocaleString();
    const joined = view?.joined;
    const server = joined?.short_name || t('setup.v5.card.second');
    const joinedText = joined ? joined.panel_domain
        ? t(joined.mail_hostname ? 'setup.v5.card.namedMail' : 'setup.v5.card.named', { server, ip: joined.ipv4, panel: joined.panel_domain, mail: joined.mail_hostname || '' })
        : t('setup.v5.card.joined', { server, ip: joined.ipv4 }) : '';
    return <section aria-labelledby="setup-join-card-title" className="mt-5 space-y-4" data-setup-join-card="">
        <h3 id="setup-join-card-title" className="sr-only">{t('setup.v5.card.title')}</h3>
        <SetupLead title={t('setup.v5.card.title')} why={t('setup.v5.card.why')} who={t('setup.v5.card.who')} next={t('setup.v5.card.next')} then={t('setup.v5.card.then')} />
        {joinedText && <p role="status" className="text-sm leading-6" data-setup-join-joined="">{joinedText}</p>}
        {made && <div className="space-y-2 rounded-lg border border-border bg-surface p-4" data-setup-join-code="">
            <SetupCopyRow label={t('setup.v5.card.address')} value={made.address} />
            <SetupCopyRow label={t('setup.v5.card.code')} value={made.code} />
            <p className="text-sm leading-6 text-fg-muted">{t('setup.v5.card.expires', { time: time(made.expires_at), left: made.attempts_left })}</p>
            <p className="text-sm leading-6 text-fg-muted">{t('setup.v5.card.shownOnce')}</p>
        </div>}
        {!made && view?.code && !view.code.used && <p className="text-sm leading-6 text-fg-muted">{t('setup.v5.card.active', { time: time(view.code.expires_at), left: view.code.attempts_left })}</p>}
        {remote.remote.state === 'unknown' && <p role="status" className="text-sm text-fg-muted">{t('setup.v5.card.readFailed')}</p>}
        {problem && <p role="alert" className="text-sm leading-6 text-danger">{problem}</p>}
        {!joined?.panel_domain && <Button type="button" variant={made ? 'secondary' : 'primary'} disabled={busy} loading={busy} onClick={() => void create()}>{t(busy ? 'setup.v5.card.creating' : made || view?.code ? 'setup.v5.card.createAgain' : 'setup.v5.card.create')}</Button>}
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

export interface SetupDelegation {
    zone: string;
    expected_ns: { name: string; server: 'main' | 'backup'; ipv4: string; ipv6?: string; glue_needed: boolean }[];
    items: { id: string; state: 'ok' | 'not_yet' | 'waiting' | 'unknown'; vars?: Record<string, string> }[];
    now_allowed: boolean;
    now_reason?: string;
    checked_at: string;
}
export function decodeSetupDelegation(value: unknown): SetupDelegation {
    const record = (item: unknown): item is Record<string, unknown> => !!item && typeof item === 'object' && !Array.isArray(item);
    const invalid = () => new Error('delegation outside the contract');
    if (!record(value) || typeof value.zone !== 'string' || !Array.isArray(value.expected_ns) || value.expected_ns.length !== 2 || !Array.isArray(value.items)
        || value.items.length > 32 || typeof value.now_allowed !== 'boolean' || typeof value.checked_at !== 'string') throw invalid();
    for (const ns of value.expected_ns) {
        if (!record(ns) || typeof ns.name !== 'string' || !['main', 'backup'].includes(String(ns.server)) || typeof ns.ipv4 !== 'string' || typeof ns.glue_needed !== 'boolean') throw invalid();
    }
    for (const item of value.items) {
        if (!record(item) || typeof item.id !== 'string' || !['ok', 'not_yet', 'waiting', 'unknown'].includes(String(item.state))) throw invalid();
    }
    return value as unknown as SetupDelegation;
}

const delegationState: Record<string, TranslationKey> = { ok: 'setup.v4.state.ok', not_yet: 'setup.v4.state.notYet', waiting: 'setup.v4.state.waiting', unknown: 'setup.v4.state.unknown' };

// The registrar card (alpha.113): the exact values, and what the internet
// shows now, read by the server about once a minute. With two servers it
// opens fully once the backup DNS server answers (or the setup waits for the
// panel's name); "do it now" is offered before that only when the main DNS
// server already answers the domain, otherwise refused with the reason.
// Alan adi firmasi karti: degerler ve internetin canli durumu.
export function SetupRegistrarCard({ draft, mainDomain, now }: { draft: ServerSetupDraft; mainDomain: string; now: boolean }) {
    const { t } = useSetupWizardI18n();
    const domain = mainDomain || draft.infrastructure_dns?.zone || '';
    const remote = useRemote(domain ? `/api/v1/setup/delegation?zone=${encodeURIComponent(domain)}` : null, decodeSetupDelegation);
    const [early, setEarly] = useState(false);
    const live = remote.remote.state === 'known' ? remote.remote.value : remote.remote.state === 'unknown' ? remote.remote.previous?.value ?? null : null;
    const primary = draft.dns_role === 'primary';
    const rows = live?.expected_ns.map(ns => ({ name: ns.name, ipv4: ns.ipv4, ipv6: ns.ipv6 || '', glue: ns.glue_needed })) ?? [
        { name: draft.ns1, ipv4: primary ? draft.local_ip : draft.peer_ip, ipv6: '', glue: !!domain && setupNameWithin(draft.ns1, domain) },
        { name: draft.ns2, ipv4: primary ? draft.peer_ip : draft.local_ip, ipv6: '', glue: !!domain && setupNameWithin(draft.ns2, domain) },
    ];
    const glue = rows.filter(row => row.glue);
    const backupReady = live?.items.some(item => item.id === 'answers_backup' && item.state === 'ok') ?? false;
    const ready = now || backupReady;
    const shown = ready || (early && !!live?.now_allowed);
    const done = !!live && live.items.length > 0 && live.items.every(item => item.state === 'ok' || (item.state !== 'unknown' && item.vars?.optional === 'true'));
    // Once everything answers correctly the card stops asking; opening the
    // step again reads once more. / Her sey dogruysa kart sormayi birakir.
    useRefreshEvery(remote, 60_000, !!domain && !done);
    const itemText = (item: SetupDelegation['items'][number]) => {
        const vars = item.vars || {};
        if (item.id.startsWith('glue_v4:') || item.id.startsWith('glue_v6:')) return t(item.id.startsWith('glue_v6:') ? 'setup.v4.item.glue6' : 'setup.v4.item.glue4', { name: vars.name || '', ip: vars.ip || '' });
        if (item.id === 'nameservers') return t('setup.v4.item.nameservers', { domain, names: vars.expected || '' });
        if (item.id === 'answers_main' || item.id === 'answers_backup') return t(item.id === 'answers_main' ? 'setup.v4.item.answersMain' : 'setup.v4.item.answersBackup', { ip: vars.ip || '' });
        if (item.id === 'imported') return t('setup.v4.item.imported', { matched: vars.matched || '0', total: vars.total || '0' });
        if (item.id === 'check_running') return t('setup.v4.item.checkRunning');
        return item.id;
    };
    const itemWhy = (item: SetupDelegation['items'][number]) => {
        const vars = item.vars || {};
        if (item.state === 'not_yet' && item.id === 'nameservers') return vars.reason === 'not_registered' ? t('setup.v4.why.notRegistered', { domain }) : vars.reason === 'no_nameservers' ? t('setup.v4.why.noNameservers', { domain }) : t('setup.v4.why.nameservers', { seen: vars.seen || '—' });
        if (item.state === 'not_yet' && item.id.startsWith('glue')) return vars.optional === 'true' ? t('setup.v4.why.glue6') : t(vars.seen ? 'setup.v4.why.glueWrong' : 'setup.v4.why.glueMissing', { seen: vars.seen || '' });
        if (item.state === 'waiting' && item.id === 'answers_backup') return t('setup.v4.why.backupWaiting');
        if (item.state === 'not_yet' && item.id === 'imported') return t('setup.v4.why.imported');
        if (item.state === 'unknown' && item.id === 'imported') return t('setup.v4.why.importedUnknown');
        return '';
    };
    return <section aria-labelledby="setup-registrar-title" className="mt-5 space-y-4" data-setup-registrar={shown ? 'now' : 'later'}>
        {done ? <SetupLead tone="done" title={t('setup.v4.doneTitle')} why={t('setup.v4.doneWhy')} who={t('setup.v2.provider.whoNone')} next={t('setup.v4.doneNext')} then={t('setup.v2.registrar.then')} />
            : <SetupLead title={t(shown ? 'setup.v2.registrar.nowTitle' : 'setup.v2.registrar.laterTitle')}
                why={t(shown ? 'setup.v2.registrar.nowWhy' : 'setup.v2.registrar.laterWhy', { domain: domain || '—', name: draft.panel_domain || '—' })}
                who={t('setup.v2.registrar.who')} next={t(glue.length ? 'setup.v2.registrar.nextGlue' : 'setup.v2.registrar.next')} then={t('setup.v2.registrar.then')} />}
        {!ready && <div className="space-y-2 text-sm leading-6">
            {!early && <Button type="button" variant="secondary" onClick={() => setEarly(true)}>{t('setup.v4.early')}</Button>}
            {early && live && !live.now_allowed && <p role="status" className="text-fg-muted" data-setup-registrar-refused="">{t('setup.v4.earlyRefused')}</p>}
            {early && !live && <p role="status" className="text-fg-muted">{t('setup.v4.reading')}</p>}
            {early && live?.now_allowed && <p className="rounded-md border border-warning-mark/40 bg-warning-mark/10 p-3">{t(/\.tr$/.test(domain) ? 'setup.v4.earlyWarnTR' : 'setup.v4.earlyWarn', { domain })}</p>}
        </div>}
        <div className="space-y-4 rounded-lg border border-border bg-surface p-4 sm:p-5">
            <h3 id="setup-registrar-title" className="font-semibold">{t('setup.v2.registrar.title')}</h3>
            {glue.length > 0 ? <div className="space-y-2">
                <h4 className="text-sm font-semibold">{t('setup.v2.registrar.glueTitle')}</h4>
                <p className="text-sm leading-6 text-fg-muted">{t('setup.v4.glueHelp', { domain })}</p>
                {glue.map(row => <div key={row.name} className="grid gap-2 sm:grid-cols-2">
                    <SetupCopyRow label={t('setup.v2.registrar.name')} value={row.name} /><SetupCopyRow label="IPv4" value={row.ipv4 || '—'} />
                    {row.ipv6 && <><span className="hidden sm:block" /><SetupCopyRow label={t('setup.v4.ipv6Optional')} value={row.ipv6} /></>}
                </div>)}
            </div> : <p className="text-sm leading-6 text-fg-muted">{t('setup.v2.registrar.noGlue')}</p>}
            <div className="space-y-2">
                <h4 className="text-sm font-semibold">{t(glue.length ? 'setup.v2.registrar.nsTitle' : 'setup.v2.registrar.nsTitleOnly')}</h4>
                <p className="text-sm leading-6 text-fg-muted">{t('setup.v2.registrar.nsHelp')}</p>
                <div className="grid gap-2 sm:grid-cols-2"><SetupCopyRow label={t('setup.v2.registrar.ns1')} value={rows[0].name} /><SetupCopyRow label={t('setup.v2.registrar.ns2')} value={rows[1].name} /></div>
            </div>
            {/\.tr$/.test(domain) && <p className="text-sm leading-6 text-fg-muted">{t('setup.v4.trNote', { domain })}</p>}
            <div className="space-y-2" aria-live="polite">
                <h4 className="text-sm font-semibold">{t('setup.v4.seenTitle')}</h4>
                {remote.remote.state === 'loading' && <p role="status" className="text-sm text-fg-muted">{t('setup.v4.reading')}</p>}
                {remote.remote.state === 'unknown' && !live && <p role="status" className="text-sm text-fg-muted">{t('setup.v4.readFailed')}</p>}
                {live && <ul className="space-y-2 text-sm">{live.items.map(item => <li key={item.id} data-delegation-item={item.id} data-state={item.state} className="flex flex-col gap-0.5">
                    <span className="flex flex-wrap items-baseline justify-between gap-x-3"><span className="break-all">{itemText(item)}</span><span className={item.state === 'ok' ? 'text-success' : 'text-fg-muted'}>{t(delegationState[item.state])}</span></span>
                    {itemWhy(item) && <span className="text-xs leading-5 text-fg-muted">{itemWhy(item)}</span>}
                </li>)}</ul>}
            </div>
            {live && <p className="text-xs text-fg-muted">{t('setup.v4.checkedAt', { time: new Date(live.checked_at).toLocaleTimeString() })}</p>}
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
    records: { name: string; type: string; values: string[]; state: 'found' | 'none' | 'unknown'; not_importable?: Record<string, string> }[];
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
interface ImportCandidate { record: SetupImportedRecord; clash: boolean; refused?: string }

// One candidate per public value. A name this setup gives to a server cannot
// keep its old address record: it is skipped and the screen says why.
function importCandidates(read: SetupPublicRecords, draft: ServerSetupDraft, own: string[]): ImportCandidate[] {
    const out: ImportCandidate[] = [];
    for (const item of read.records) {
        if (item.state !== 'found' || !importTypes.includes(item.type as ImportType)) continue;
        for (const value of item.values) {
            // A value the review would refuse (the zone renderer's own rule) is
            // shown with its reason and never sent: it cannot trap the draft.
            const refused = item.not_importable?.[value];
            if (refused) { out.push({ clash: false, refused, record: { name: item.name, type: item.type as ImportType, content: value, decision: 'skip' } }); continue; }
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
    const wanted = JSON.stringify(candidates.filter(item => !item.refused).map(item => item.record));
    useEffect(() => {
        if (!read) return;
        update(previous => JSON.stringify(previous.imported_records || []) === wanted ? previous : { ...previous, imported_records: JSON.parse(wanted) });
        // `wanted` is the whole decision list; the read itself only changes it.
    }, [wanted]);
    const decide = (index: number, decision: 'import' | 'skip') => update(previous => {
        const list = candidates.map(item => item.record);
        list[index] = { ...list[index], decision };
        return { ...previous, imported_records: list.filter((_, position) => !candidates[position].refused) };
    });
    const unread = read?.records.filter(item => item.state === 'unknown') || [];
    const imported = candidates.filter(item => !item.refused && item.record.decision === 'import').length;
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
                        {item.refused ? <span className="text-fg-muted" data-not-importable={item.refused}>{t('setup.v3.records.notImportable')}</span>
                            : item.clash ? <span className="text-fg-muted">{t('setup.v3.records.clash')}</span>
                            : <span role="radiogroup" aria-label={record.name} className="flex gap-3">
                                {(['import', 'skip'] as const).map(choice => <label key={choice} className="inline-flex cursor-pointer items-center gap-1.5"><input type="radio" name={`setup-import-${index}`} value={choice} checked={record.decision === choice} onChange={() => decide(index, choice)} className="h-4 w-4 accent-primary" />{t(choice === 'import' ? 'setup.v3.records.import' : 'setup.v3.records.skip')}</label>)}
                            </span>}
                    </div>
                    {item.refused && <p className="leading-6 text-fg-muted">{t(item.refused === 'one_per_name' ? 'setup.v3.records.onePerName' : item.refused === 'too_many' ? 'setup.v3.records.tooMany' : 'setup.v3.records.invalid')}</p>}
                    {item.clash && <p className="leading-6 text-fg-muted">{t('setup.v3.records.clashHelp', { name: record.name })}</p>}
                    {!item.clash && !item.refused && record.decision === 'skip' && <p className="leading-6 text-fg-muted">{t('setup.v3.records.skipHelp')}</p>}
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
    // The shown default is saved, so the server applies exactly what the page says.
    useEffect(() => {
        if (askMX && !draft.mail_dns?.mx) update(previous => previous.mail_dns?.mx ? previous : { ...previous, mail_dns: { ...previous.mail_dns, mx: 'later' } });
    }, [askMX]);
    const spf = mergeSetupSPF(setupImportedSPF(draft, mainDomain), host);
    return <section className="space-y-5" aria-labelledby="setup-mail-title" data-setup-mail="">
        <h2 id="setup-mail-title" className="text-xl font-semibold">{t('setup.v3.mail.title')}</h2>
        <p className="max-w-2xl text-sm leading-6 text-fg-muted">{t(local ? 'setup.v3.mail.helpLocal' : draft.dns_mode === 'external' ? 'setup.v3.mail.helpExternal' : draft.dns_mode === 'local' ? 'setup.v3.mail.helpBackup' : 'setup.v3.mail.helpElsewhere', { domain: mainDomain })}</p>
        <ul className="space-y-2 text-sm">
            <li className="rounded-md border border-border bg-surface-2 px-3 py-2"><span className="font-medium">MX</span> · <span className="break-all font-mono">{mx === 'now' ? `10 ${host}` : t('setup.v3.mail.mxKept', { old: oldMX.map(item => item.content).join(', ') || '—' })}</span></li>
            <li className="rounded-md border border-border bg-surface-2 px-3 py-2"><span className="font-medium">SPF</span> · <span className="break-all font-mono">{spf}</span>{setupImportedSPF(draft, mainDomain) && <span className="block text-xs text-fg-muted">{t('setup.v3.mail.spfMerged')}</span>}</li>
            <li className="rounded-md border border-border bg-surface-2 px-3 py-2"><span className="font-medium">DKIM</span> · {t(local ? 'setup.v3.mail.dkim' : draft.dns_mode === 'external' ? 'setup.v3.mail.dkimExternal' : 'setup.v3.mail.dkimElsewhere', { domain: mainDomain })}</li>
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
            <p className="text-fg-muted">{t('setup.v3.mail.ptrOpenTask')}</p>
            <p className="text-fg-muted">{t('setup.v3.mail.port25')}</p>
        </div>
    </section>;
}

// Open tasks setup does not wait for (reverse DNS at the server provider),
// said in D-024 order: why, who acts, the next action, then.
export function SetupOpenTasks({ lines }: { lines: string[] }) {
    const { t } = useSetupWizardI18n();
    return <div className="mt-5" data-setup-open-tasks="">
        <SetupLead title={t('setup.v3.tasks.title')} why={t('setup.v3.tasks.why')} who={t('setup.v3.tasks.who')}
            next={<ul className="list-disc space-y-1 pl-5">{lines.map((line, index) => <li key={index}>{line}</li>)}</ul>} then={t('setup.v3.tasks.then')} />
    </div>;
}
