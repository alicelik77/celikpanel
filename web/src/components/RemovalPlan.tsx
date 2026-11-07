import { Link } from '../router';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Dialog, KeepWords } from './ui';
import { DownloadCloud } from 'lucide-react';

// The component dependency rule on screen (alpha.123, D-045). The Panel
// answers "what requires the thing being removed?" with one plan
// (cmd/panel/component_removal_plan.go); this file shows it in three groups:
// what must be moved or removed first (each with a link to where), what is
// removed together (ticked; keeping them means the component cannot be
// removed) and what stops. "Remove together" runs on the server as one
// durable operation; the window only follows it. The install side shows the
// requirements a component still needs, with a button for each.
// Bileşen bağımlılık kuralı ekranda: önce taşınacak/kaldırılacaklar,
// birlikte kaldırılacaklar (işaretli), duracak olanlar.

export interface RemovalPlanItem { name: string; link?: string }
export interface RemovalBlocker { kind: string; component?: string; count: number; items: RemovalPlanItem[] }
export interface RemovalPlan {
    blockers: RemovalBlocker[];
    together: string[];
    stops: { kind: string; count: number }[];
    withdraws: string[];
    withdraw_sets: number;
    notes: string[];
}

const isStrings = (value: unknown): value is string[] => Array.isArray(value) && value.every((item) => typeof item === 'string');
const isCount = (value: unknown): value is number => typeof value === 'number' && Number.isInteger(value) && value >= 0;
// Only links inside the panel are followed. / Yalnız panel içi bağlantılar.
const safeLink = (value: unknown) => (typeof value === 'string' && value.startsWith('/') && !value.startsWith('//') ? value : undefined);

export function decodeRemovalPlan(value: unknown): RemovalPlan | null {
    if (!value || typeof value !== 'object') return null;
    const v = value as Record<string, unknown>;
    if (!Array.isArray(v.blockers) || !isStrings(v.together) || !Array.isArray(v.stops) || !isStrings(v.withdraws) || !isCount(v.withdraw_sets)) return null;
    const blockers: RemovalBlocker[] = [];
    for (const raw of v.blockers) {
        const b = raw as Record<string, unknown> | null;
        if (!b || typeof b.kind !== 'string' || !isCount(b.count) || !Array.isArray(b.items)) return null;
        const items: RemovalPlanItem[] = [];
        for (const item of b.items) {
            const i = item as Record<string, unknown> | null;
            if (!i || typeof i.name !== 'string') return null;
            items.push({ name: i.name, link: safeLink(i.link) });
        }
        blockers.push({ kind: b.kind, ...(typeof b.component === 'string' ? { component: b.component } : {}), count: b.count, items });
    }
    const stops: RemovalPlan['stops'] = [];
    for (const raw of v.stops) {
        const s = raw as Record<string, unknown> | null;
        if (!s || typeof s.kind !== 'string' || !isCount(s.count)) return null;
        stops.push({ kind: s.kind, count: s.count });
    }
    return { blockers, together: v.together, stops, withdraws: v.withdraws, withdraw_sets: v.withdraw_sets, notes: isStrings(v.notes) ? v.notes : [] };
}

// A removal the plan refuses: something must go first, or the owner keeps
// what can only go together with it.
export function removalPlanRefused(plan: RemovalPlan | null, keepTogether = false): boolean {
    return plan !== null && (plan.blockers.length > 0 || (keepTogether && plan.together.length > 0));
}

// The components of the together list (settings are said, not removed).
export const togetherComponents = (plan: RemovalPlan | null) => (plan?.together ?? []).filter((item) => !item.startsWith('setting:'));

const BLOCKER_KINDS = new Set(['component', 'sites', 'apache_sites', 'php_sites', 'php_tools', 'node_sites', 'node_service_apps', 'python_apps', 'databases',
    'mail_addresses', 'dns_zones', 'certificates', 'panel_firewall', 'panel_firewall_unknown', 'vpn_peers', 'malware_quarantine']);
const STOP_KINDS = new Set(['webmail', 'spam_filtering', 'ban_protection', 'malware_scans', 'ftp', 'monitoring', 'unknown_use', 'toolchain', 'database_tool', 'outgoing_limits']);

const groupTitle = 'font-semibold text-fg';

function BlockerItems({ blocker, nameOf, onRemoveComponent }: { blocker: RemovalBlocker; nameOf: (id: string) => string; onRemoveComponent?: (id: string) => void }) {
    const { t } = useI18n();
    if (blocker.kind === 'component' && blocker.component) {
        const id = blocker.component;
        return onRemoveComponent ? (
            <button type="button" onClick={() => onRemoveComponent(id)} data-rmrule-remove={id}
                className="font-medium text-primary underline underline-offset-2 hover:no-underline max-sm:min-h-11">
                {t('rmrule.removeFirst', { name: nameOf(id) })}
            </button>
        ) : <Link to="/components" className="text-primary underline">{t('rmrule.removeFirst', { name: nameOf(id) })}</Link>;
    }
    const more = blocker.count - blocker.items.length;
    return (
        <ul className="mt-1 space-y-0.5 pl-4">
            {blocker.items.map((item, index) => (
                <li key={`${item.name}-${index}`} data-rmrule-item={item.name}>
                    {item.link ? <Link to={item.link} className="text-primary underline">{item.name}</Link> : <span className="text-fg">{item.name}</span>}
                </li>
            ))}
            {more > 0 && <li className="text-fg-muted">{t('rmrule.more', { n: more })}</li>}
        </ul>
    );
}

// The three groups. keepTogether: the owner unticked the together list.
export function RemovalPlanGroups({ plan, name, nameOf, keepTogether, onKeepTogether, busy, onRemoveComponent }: {
    plan: RemovalPlan;
    name: string;
    nameOf: (id: string) => string;
    keepTogether: boolean;
    onKeepTogether: (keep: boolean) => void;
    busy: boolean;
    onRemoveComponent?: (id: string) => void;
}) {
    const { t } = useI18n();
    const blockers = plan.blockers.filter((b) => BLOCKER_KINDS.has(b.kind));
    const stops = plan.stops.filter((s) => STOP_KINDS.has(s.kind));
    const together = plan.together.map((item) => item.startsWith('setting:') ? t(`rmrule.setting.${item.slice(8)}` as TranslationKey) : nameOf(item));
    return (
        <div className="space-y-3 text-sm" data-rmrule="">
            {blockers.length > 0 && (
                <div className="rounded-lg border border-warning-mark/50 bg-warning-mark/20 px-3 py-2" data-rmrule-group="first">
                    <p className={groupTitle}>{t('rmrule.firstTitle')}</p>
                    <ul className="mt-1 list-disc space-y-2 pl-5 text-fg">
                        {blockers.map((blocker) => (
                            <li key={`${blocker.kind}-${blocker.component ?? ''}`} data-rmrule-blocker={blocker.component ? `component:${blocker.component}` : blocker.kind}>
                                <KeepWords text={t(`rmrule.kind.${blocker.kind}` as TranslationKey, { count: blocker.count, name: blocker.component ? nameOf(blocker.component) : name })} />
                                <BlockerItems blocker={blocker} nameOf={nameOf} onRemoveComponent={onRemoveComponent} />
                            </li>
                        ))}
                    </ul>
                    <p className="mt-2 text-fg-muted">{t('rmrule.next', { name })}</p>
                </div>
            )}
            {plan.notes.includes('runtime_units_unknown') && <p className="text-fg-muted" data-rmrule-note="units">{t('rmrule.unitsUnknown')}</p>}
            {together.length > 0 && (
                <div data-rmrule-group="together">
                    <p className={groupTitle}>{t('rmrule.togetherTitle')}</p>
                    <label className="mt-1 flex items-start gap-2 text-fg" data-removal-companions={togetherComponents(plan).join(' ')}>
                        <input type="checkbox" className="mt-1" checked={!keepTogether} disabled={busy} onChange={(event) => onKeepTogether(!event.target.checked)} data-rmrule-together="" />
                        <span><KeepWords text={together.join(', ')} /> <span className="text-fg-muted">{t('rmrule.togetherHint', { name })}</span></span>
                    </label>
                    {keepTogether && <p className="mt-1 text-fg" data-rmrule-kept="">{t('rmrule.togetherKeep', { name })}</p>}
                </div>
            )}
            {blockers.length === 0 && (
                <div data-rmrule-group="stops">
                    <p className={groupTitle}>{t('rmrule.stopsTitle')}</p>
                    {stops.length === 0
                        ? <p className="mt-1 text-fg-muted" data-rmrule-stop="none">{t('rmrule.nothingStops')}</p>
                        : (
                            <ul className="mt-1 list-disc space-y-1 pl-5 text-fg">
                                {stops.map((stop) => <li key={stop.kind} data-rmrule-stop={stop.kind}>{t(`rmrule.stop.${stop.kind}` as TranslationKey, { count: stop.count })}</li>)}
                            </ul>
                        )}
                    {plan.withdraws.length > 0 && <p className="mt-1 text-fg-muted" data-rmrule-withdraws={plan.withdraws.length}>{t('rmrule.withdraws', { count: plan.withdraws.length, sets: plan.withdraw_sets })}</p>}
                </div>
            )}
        </div>
    );
}

// --- install: requirements first ------------------------------------------

export interface InstallNeedsService { id: string; name: string; is_installed: boolean | null; provides?: string[]; not_offered?: boolean; conflict_with?: string }

// What a component still needs, each with the way to install it: a
// component by its own button, a role by the components that fill it.
export function InstallNeedsDialog({ service, missing, services, roleName, onInstall, onCancel }: {
    service: InstallNeedsService;
    missing: string[];
    services: InstallNeedsService[];
    roleName: (token: string) => string | null;
    onInstall: (id: string) => void;
    onCancel: () => void;
}) {
    const { t } = useI18n();
    const options = (token: string) => {
        const role = roleName(token);
        const ids = role === null ? [token] : services.filter((s) => s.provides?.includes(token)).map((s) => s.id);
        return { role, choices: services.filter((s) => ids.includes(s.id) && s.is_installed !== true && !s.not_offered && !s.conflict_with) };
    };
    return (
        <Dialog id="service-install-needs" icon={DownloadCloud} title={t('rmrule.install.title', { name: service.name })}
            description={t('rmrule.install.why', { name: service.name })} onDismiss={onCancel}
            actions={<Button variant="secondary" onClick={onCancel}>{t('common.cancel')}</Button>}>
            <ul className="space-y-3 text-sm" data-install-needs={missing.join(' ')}>
                {missing.map((token) => {
                    const { role, choices } = options(token);
                    return (
                        <li key={token} data-install-need={token}>
                            <p className="font-medium text-fg">{role ?? services.find((s) => s.id === token)?.name ?? token}</p>
                            {role !== null && <p className="text-fg-muted">{t('rmrule.install.role', { role })}</p>}
                            {choices.length === 0
                                ? <p className="text-fg-muted" data-install-need-none="">{t('rmrule.install.none')}</p>
                                : (
                                    <div className="mt-1 flex flex-wrap gap-2">
                                        {choices.map((choice) => (
                                            <Button key={choice.id} variant="secondary" icon={DownloadCloud} onClick={() => onInstall(choice.id)} data-install-first={choice.id}>
                                                {t('rmrule.install.first', { name: choice.name })}
                                            </Button>
                                        ))}
                                    </div>
                                )}
                        </li>
                    );
                })}
            </ul>
            <p className="mt-3 text-sm text-fg-muted">{t('rmrule.install.next', { name: service.name })}</p>
        </Dialog>
    );
}
