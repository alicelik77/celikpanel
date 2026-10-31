import { useState } from 'react';
import { Users, Plus, Trash2, LogIn, Pause, Play, Pencil, Save, X, Layers, Gauge } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { useAuth } from '../auth/AuthContext';
import { type PanelUser, type ServicePlan } from '../lib/api';
import { Button, CouldNotCheck, KnownEmpty, RemoteGate, StatusDot, inputClass } from './ui';
import { PageHeader } from './PageHeader';
import { readApiError, apiErrorText } from '../lib/apiError';
import { countText, mapRemote, useRemote } from '../lib/remote';
import { PLANS_URL, USERS_URL, decodePlans, decodeUsers } from '../lib/accounts';
import { TeamMembersPage } from './TeamMembersPage';

// Account management: the admin/reseller view over the role hierarchy.
// Everything here mirrors what the API enforces — role options, visibility
// and quota errors all come from the server.
//
// Hesap yönetimi: rol hiyerarşisi üzerinde admin/bayi görünümü. Buradaki her
// şey API'nin uyguladığını yansıtır — rol seçenekleri, görünürlük ve kota
// hataları sunucudan gelir.
export function UsersPage() {
    const { role, user } = useAuth();
    if (role === 'customer') {
        if (user?.account_type !== 'account' || user.features.team_members !== true) return null;
        return <TeamMembersPage />;
    }
    if (role !== 'admin' && role !== 'reseller') return null;
    return <AccountUsersPage role={role} />;
}

function AccountUsersPage({ role }: { role: 'admin' | 'reseller' }) {
    const { t } = useI18n();
    const isAdmin = role === 'admin';
    const [tab, setTab] = useState<'accounts' | 'plans'>('accounts');

    return (
        <div className="p-6 md:p-8">
            <PageHeader
                title={t('nav.users')}
                subtitle={t('users.subtitle')}
                breadcrumb={[t('common.home'), t('nav.users')]}
            />

            <div className="mb-4 flex items-center gap-1 border-b border-border">
                <Tab active={tab === 'accounts'} onClick={() => setTab('accounts')} label={t('users.tab.accounts')} />
                {/* Resellers have their own plans too (alpha.114). / Bayilerin de kendi planları var. */}
                <Tab active={tab === 'plans'} onClick={() => setTab('plans')} label={t('users.tab.plans')} />
            </div>

            {tab === 'accounts' ? <AccountsTab isAdmin={isAdmin} /> : <PlansTab isAdmin={isAdmin} />}
        </div>
    );
}

function AccountsTab({ isAdmin }: { isAdmin: boolean }) {
    const { t } = useI18n();
    const users = useRemote(USERS_URL, decodeUsers);
    const plans = useRemote(PLANS_URL, decodePlans);
    const [showForm, setShowForm] = useState(false);

    const [username, setUsername] = useState('');
    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [newRole, setNewRole] = useState('customer');
    const [planID, setPlanID] = useState(0);
    const [saving, setSaving] = useState(false);
    // One row's plan or pool panel is open at a time (alpha.114).
    // Aynı anda tek satırın plan ya da havuz paneli açıktır.
    const [panel, setPanel] = useState<{ user: PanelUser; kind: 'plan' | 'pool' } | null>(null);

    const load = () => {
        void users.retry();
        void plans.retry();
    };
    // An account is created with one of the plans the server named, and a row
    // is changed as the server last listed it. Neither is offered on a list
    // that is being read again or could not be read.
    // Hesap, sunucunun adlandırdığı planlardan biriyle oluşturulur; satır da
    // sunucunun son listelediği hâliyle değiştirilir. Liste yeniden okunurken
    // ya da okunamadığında ikisi de sunulmaz.
    const usersKnown = users.remote.state === 'known' && !users.reading;
    const plansKnown = plans.remote.state === 'known';
    const planOptions = plans.remote.state === 'known' ? plans.remote.value : [];

    // Conflict answers (quota, children, duplicates) carry real reasons from
    // the API; the coded contract picks a localized text when the refusal
    // has a code, else the server message — never a vague generic.
    // Çakışma yanıtları (kota, alt hesap, mükerrer) API'den gerçek nedenlerle
    // gelir; kodlu sözleşme, ret kodluysa yerelleştirilmiş metni, değilse
    // sunucu mesajını seçer — asla belirsiz bir genel değil.
    const apiError = async (res: Response) => {
        showToast('error', apiErrorText(await readApiError(res), t));
    };
    // The answer to a change did not arrive: it is not known whether it was
    // made. Nothing is sent again; the list is read so the person can look.
    // Değişikliğin yanıtı gelmedi: yapılıp yapılmadığı bilinmiyor. Hiçbir şey
    // yeniden gönderilmez; kişi bakabilsin diye liste okunur.
    const send = async (url: string, init: RequestInit): Promise<Response | null> => {
        try {
            return await fetch(url, init);
        } catch {
            showToast('error', t('common.resultUnknown'));
            load();
            return null;
        }
    };

    const createUser = async () => {
        setSaving(true);
        try {
            const body: Record<string, unknown> = { username, email, password, role: newRole };
            if (planID > 0) body.plan_id = planID;
            const res = await send('/api/v1/users', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            });
            if (!res) return;
            if (!res.ok) {
                await apiError(res);
                return;
            }
            showToast('success', t('users.created'));
            setShowForm(false);
            setUsername('');
            setEmail('');
            setPassword('');
            load();
        } finally {
            setSaving(false);
        }
    };

    const setStatus = async (u: PanelUser, status: 'active' | 'suspended') => {
        if (status === 'suspended' && !confirm(t('users.suspendConfirm', { name: u.username }))) return;
        const res = await send(`/api/v1/users/${u.id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ status }),
        });
        if (!res) return;
        if (!res.ok) {
            await apiError(res);
            return;
        }
        showToast('success', status === 'suspended' ? t('users.suspended') : t('users.activated'));
        load();
    };

    const deleteUser = async (u: PanelUser) => {
        if (!confirm(t('users.deleteConfirm', { name: u.username }))) return;
        const res = await send(`/api/v1/users/${u.id}`, { method: 'DELETE' });
        if (!res) return;
        if (!res.ok) {
            await apiError(res);
            return;
        }
        showToast('success', t('users.deleted'));
        load();
    };

    const impersonate = async (u: PanelUser) => {
        const res = await send(`/api/v1/users/${u.id}/impersonate`, { method: 'POST' });
        if (!res) return;
        if (!res.ok) {
            await apiError(res);
            return;
        }
        // Full reload: the whole shell re-derives from the new session.
        // Tam yenileme: kabuk yeni oturumdan baştan türesin.
        window.location.assign('/');
    };

    const roleKey = (r: string): TranslationKey =>
        r === 'admin' ? 'users.role.admin' : r === 'reseller' ? 'users.role.reseller' : 'users.role.customer';

    return (
        <div>
            <div className="mb-3 flex items-center justify-between">
                <span className="text-xs text-fg-subtle">{(countText(mapRemote(users.remote, (rows) => rows.length)) === '1' ? t('common.itemsTotalOne') : t('common.itemsTotal', { n: countText(mapRemote(users.remote, (rows) => rows.length)) }))}</span>
                <Button variant="primary" icon={Plus} disabled={!usersKnown || !plansKnown} onClick={() => setShowForm((s) => !s)}>
                    {t('users.add')}
                </Button>
            </div>

            {plans.remote.state === 'unknown' && (
                <CouldNotCheck
                    className="mb-4"
                    text={t('users.plansUnknown')}
                    onRetry={() => void plans.retry()}
                    busy={plans.reading}
                />
            )}

            {showForm && plansKnown && (
                <div className="mb-4 rounded-xl border border-border bg-surface-2/50 p-4">
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5">
                        <label>
                            <span className="mb-1 block text-xs text-fg-muted">{t('users.form.username')}</span>
                            <input value={username} onChange={(e) => setUsername(e.target.value)} className={inputClass} autoFocus />
                        </label>
                        <label>
                            <span className="mb-1 block text-xs text-fg-muted">{t('users.form.email')}</span>
                            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} className={inputClass} />
                        </label>
                        <label>
                            <span className="mb-1 block text-xs text-fg-muted">{t('users.form.password')}</span>
                            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} className={inputClass} />
                        </label>
                        <label>
                            <span className="mb-1 block text-xs text-fg-muted">{t('users.form.role')}</span>
                            <select
                                value={newRole}
                                onChange={(e) => setNewRole(e.target.value)}
                                className={inputClass}
                                disabled={!isAdmin}
                            >
                                <option value="customer">{t('users.role.customer')}</option>
                                {isAdmin && <option value="reseller">{t('users.role.reseller')}</option>}
                            </select>
                        </label>
                        <label>
                            <span className="mb-1 block text-xs text-fg-muted">{t('users.form.plan')}</span>
                            <select value={planID} onChange={(e) => setPlanID(Number(e.target.value))} className={inputClass}>
                                <option value={0}>{t('users.form.noPlan')}</option>
                                {planOptions.filter((p) => p.owner_id === undefined || !isAdmin).map((p) => (
                                    <option key={p.id} value={p.id}>
                                        {p.name}
                                    </option>
                                ))}
                            </select>
                        </label>
                    </div>
                    <div className="mt-3 flex justify-end gap-2">
                        <Button onClick={() => setShowForm(false)}>{t('users.cancel')}</Button>
                        <Button
                            variant="primary"
                            icon={Plus}
                            onClick={createUser}
                            disabled={saving || !usersKnown || !username || !email || password.length < 8}
                        >
                            {t('users.create')}
                        </Button>
                    </div>
                </div>
            )}

            {panel?.kind === 'plan' && (
                <PlanAssignPanel
                    user={panel.user}
                    plans={planOptions.filter((p) => p.owner_id === undefined || panel.user.parent_id === p.owner_id)}
                    onClose={() => setPanel(null)}
                    onDone={() => {
                        setPanel(null);
                        load();
                    }}
                />
            )}
            {panel?.kind === 'pool' && <PoolPanel user={panel.user} onClose={() => setPanel(null)} />}

            <RemoteGate
                remote={users.remote}
                checking={t('users.checking')}
                failed={t('users.unknown')}
                onRetry={() => void users.retry()}
                busy={users.reading}
                className="py-3"
            >
                {(shown) => (shown.value.length === 0 ? (
                    <KnownEmpty of={shown} icon={Users} title={t('users.empty')} hint={t('users.emptyHint')} />
                ) : (
                    <div className="overflow-x-auto rounded-xl border border-border-strong bg-surface">
                        <table className="w-full text-sm">
                            <thead>
                                <tr className="border-b border-border text-left text-xs font-semibold text-fg-muted">
                                    <th className="px-4 py-2.5">{t('users.col.user')}</th>
                                    <th className="px-4 py-2.5">{t('users.col.role')}</th>
                                    <th className="px-4 py-2.5">{t('users.col.status')}</th>
                                    {isAdmin && <th className="px-4 py-2.5">{t('users.col.parent')}</th>}
                                    <th className="px-4 py-2.5">{t('users.col.usage')}</th>
                                    <th className="px-4 py-2.5" />
                                </tr>
                            </thead>
                            <tbody>
                                {shown.value.map((u) => (
                                    <tr key={u.id} className="border-b border-border last:border-0 hover:bg-surface-2/60">
                                        <td className="px-4 py-3">
                                            <div className="text-base font-medium text-fg">{u.username}</div>
                                            <div className="text-xs text-fg-subtle">{u.email}</div>
                                        </td>
                                        <td className="px-4 py-3">
                                            <span
                                                className={`rounded-md px-2 py-0.5 text-xs font-medium ${
                                                    u.role === 'admin'
                                                        ? 'bg-danger/10 text-danger'
                                                        : u.role === 'reseller'
                                                          ? 'bg-warning/15 text-warning'
                                                          : 'bg-primary/10 text-primary'
                                                }`}
                                            >
                                                {t(roleKey(u.role))}
                                            </span>
                                        </td>
                                        <td className="px-4 py-3">
                                            <span className="inline-flex items-center gap-1.5 text-fg-muted">
                                                <StatusDot ok={u.status === 'active'} />
                                                {u.status === 'active' ? t('users.status.active') : t('users.status.suspended')}
                                            </span>
                                        </td>
                                        {isAdmin && <td className="px-4 py-3 text-fg-muted">{u.parent_name || '—'}</td>}
                                        <td className="px-4 py-3 text-fg-muted">
                                            {u.subscriptions} / {u.domains}
                                        </td>
                                        <td className="px-4 py-3">
                                            {u.role !== 'admin' && (
                                                <div className="flex items-center justify-end gap-0.5">
                                                    <RowBtn disabled={!usersKnown || !plansKnown} title={t('users.changePlan')} onClick={() => setPanel({ user: u, kind: 'plan' })}>
                                                        <Layers className="h-4 w-4" />
                                                    </RowBtn>
                                                    {isAdmin && u.role === 'reseller' && (
                                                        <RowBtn disabled={!usersKnown} title={t('users.pool')} onClick={() => setPanel({ user: u, kind: 'pool' })}>
                                                            <Gauge className="h-4 w-4" />
                                                        </RowBtn>
                                                    )}
                                                    <RowBtn disabled={!usersKnown} title={t('users.loginAs')} onClick={() => impersonate(u)}>
                                                        <LogIn className="h-4 w-4" />
                                                    </RowBtn>
                                                    {u.status === 'active' ? (
                                                        <RowBtn disabled={!usersKnown} title={t('users.suspend')} onClick={() => setStatus(u, 'suspended')}>
                                                            <Pause className="h-4 w-4" />
                                                        </RowBtn>
                                                    ) : (
                                                        <RowBtn disabled={!usersKnown} title={t('users.activate')} onClick={() => setStatus(u, 'active')}>
                                                            <Play className="h-4 w-4" />
                                                        </RowBtn>
                                                    )}
                                                    <RowBtn danger disabled={!usersKnown} title={t('users.delete')} onClick={() => deleteUser(u)}>
                                                        <Trash2 className="h-4 w-4" />
                                                    </RowBtn>
                                                </div>
                                            )}
                                        </td>
                                    </tr>
                ))}
                        </tbody>
                    </table>
                </div>
            ))}
            </RemoteGate>
        </div>
    );
}

// PlanAssignPanel gives an account a plan or changes it (alpha.114). A plan
// below current usage is refused first with the list of what is over; the
// operator may then apply it anyway — nothing is deleted, and new items of
// those kinds are refused until usage is under the limit.
// PlanAssignPanel, hesaba plan verir ya da değiştirir. Kullanımın altındaki
// plan önce aşan kalemlerin listesiyle reddedilir; operatör yine de
// uygulayabilir — hiçbir şey silinmez.
function PlanAssignPanel({ user, plans, onClose, onDone }: { user: PanelUser; plans: ServicePlan[]; onClose: () => void; onDone: () => void }) {
    const { t } = useI18n();
    const [planID, setPlanID] = useState(plans[0]?.id ?? 0);
    const [busy, setBusy] = useState(false);
    const [over, setOver] = useState<string[] | null>(null);

    const apply = async (accept: boolean) => {
        setBusy(true);
        try {
            const res = await fetch(`/api/v1/users/${user.id}/plan`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(accept ? { plan_id: planID, accept_over_usage: true } : { plan_id: planID }),
            });
            if (!res.ok) {
                const err = await readApiError(res);
                if (err.code === 'PLAN_BELOW_USAGE') {
                    setOver(err.details ?? []);
                    return;
                }
                showToast('error', apiErrorText(err, t));
                return;
            }
            showToast('success', accept ? t('users.planAppliedOver') : t('users.planApplied'));
            onDone();
        } catch {
            // The answer did not arrive: it is not known whether the plan
            // changed. Nothing is sent again; the list is read again.
            showToast('error', t('common.resultUnknown'));
            onDone();
        } finally {
            setBusy(false);
        }
    };

    return (
        <div className="mb-4 rounded-xl border border-border bg-surface-2/50 p-4" data-plan-assign={user.id}>
            <p className="text-sm font-semibold text-fg">{t('users.planFor', { name: user.username })}</p>
            {plans.length === 0 ? (
                <p className="mt-2 text-sm text-fg-muted">{t('users.planNone')}</p>
            ) : (
                <label className="mt-3 block max-w-sm">
                    <span className="mb-1 block text-xs text-fg-muted">{t('users.form.plan')}</span>
                    <select value={planID} onChange={(e) => { setPlanID(Number(e.target.value)); setOver(null); }} className={inputClass}>
                        {plans.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
                    </select>
                </label>
            )}
            {over && (
                <div className="mt-3 rounded-md bg-danger/10 p-3 text-sm text-danger" role="alert">
                    <p className="font-semibold">{t('err.PLAN_BELOW_USAGE')}</p>
                    <ul className="mt-1 list-disc pl-5">{over.map((line) => <li key={line}>{line}</li>)}</ul>
                </div>
            )}
            <div className="mt-3 flex flex-wrap justify-end gap-2">
                <Button icon={X} onClick={onClose}>{t('users.cancel')}</Button>
                {over ? (
                    <Button variant="primary" onClick={() => apply(true)} disabled={busy}>{t('users.planApplyAnyway')}</Button>
                ) : (
                    <Button variant="primary" icon={Save} onClick={() => apply(false)} disabled={busy || planID === 0}>{t('users.planApply')}</Button>
                )}
            </div>
        </div>
    );
}

type PoolForm = { max_customers: number; max_domains: number; max_databases: number; max_email_accounts: number; disk_quota_mb: number };

// PoolPanel: the administrator sets what a reseller may hand out (alpha.114).
// Each 0 is unlimited. A pool below what is already handed out is saved with
// a warning: nothing is taken away; new allocations in that tree are refused.
// PoolPanel: yönetici, bayinin dağıtabileceğini belirler. Her 0 sınırsızdır.
function PoolPanel({ user, onClose }: { user: PanelUser; onClose: () => void }) {
    const { t } = useI18n();
    const url = `/api/v1/users/${user.id}/pool`;
    const read = useRemote(url, (raw: unknown) => {
        if (!raw || typeof raw !== 'object' || !('allocated' in raw)) throw new Error('shape');
        return raw as { pool: PoolForm | null; allocated: Record<string, number | boolean>; over_limit: string[] };
    });
    const [form, setForm] = useState<PoolForm | null>(null);
    const [busy, setBusy] = useState(false);
    const known = read.remote.state === 'known' ? read.remote.value : null;
    const shown: PoolForm = form ?? known?.pool ?? { max_customers: 0, max_domains: 0, max_databases: 0, max_email_accounts: 0, disk_quota_mb: 0 };

    const save = async (pool: PoolForm | null) => {
        setBusy(true);
        try {
            const res = await fetch(url, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ pool }) });
            if (!res.ok) {
                showToast('error', apiErrorText(await readApiError(res), t));
                return;
            }
            const answer = (await res.json()) as { over_limit?: string[] };
            showToast('success', answer.over_limit && answer.over_limit.length > 0 ? t('users.poolSavedOver') : t('users.poolSaved'));
            setForm(null);
            void read.retry();
        } catch {
            showToast('error', t('common.resultUnknown'));
            void read.retry();
        } finally {
            setBusy(false);
        }
    };

    return (
        <div className="mb-4 rounded-xl border border-border bg-surface-2/50 p-4" data-pool-panel={user.id}>
            <p className="text-sm font-semibold text-fg">{t('users.poolFor', { name: user.username })}</p>
            <p className="mt-1 text-xs text-fg-subtle">{t('users.poolHint')}</p>
            <RemoteGate remote={read.remote} checking={t('users.poolChecking')} failed={t('users.poolUnknown')} onRetry={() => void read.retry()} busy={read.reading} className="py-3">
                {(state) => (
                    <>
                        {state.value.over_limit.length > 0 && (
                            <ul className="mt-2 list-disc pl-5 text-sm text-danger">{state.value.over_limit.map((line) => <li key={line}>{line}</li>)}</ul>
                        )}
                        <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
                            <NumField label={t('myPlan.customers')} value={shown.max_customers} onChange={(v) => setForm({ ...shown, max_customers: v })} />
                            <NumField label={t('plans.col.domains')} value={shown.max_domains} onChange={(v) => setForm({ ...shown, max_domains: v })} />
                            <NumField label={t('plans.col.databases')} value={shown.max_databases} onChange={(v) => setForm({ ...shown, max_databases: v })} />
                            <NumField label={t('plans.col.mail')} value={shown.max_email_accounts} onChange={(v) => setForm({ ...shown, max_email_accounts: v })} />
                            <NumField label={`${t('plans.col.disk')} (MB)`} value={shown.disk_quota_mb} onChange={(v) => setForm({ ...shown, disk_quota_mb: v })} />
                        </div>
                        <p className="mt-2 text-xs text-fg-subtle">
                            {t('users.poolAllocated', {
                                customers: String(state.value.allocated.customers ?? 0),
                                domains: state.value.allocated.unlimited_domains ? '∞' : String(state.value.allocated.domains ?? 0),
                                disk: state.value.allocated.unlimited_disk ? '∞' : String(state.value.allocated.disk_mb ?? 0),
                            })}
                        </p>
                    </>
                )}
            </RemoteGate>
            <div className="mt-3 flex flex-wrap justify-end gap-2">
                <Button icon={X} onClick={onClose}>{t('users.cancel')}</Button>
                {known?.pool && <Button onClick={() => save(null)} disabled={busy}>{t('users.poolRemove')}</Button>}
                <Button variant="primary" icon={Save} onClick={() => save(shown)} disabled={busy || !known}>{t('plans.save')}</Button>
            </div>
        </div>
    );
}

// A new plan starts with modest limits; 0 means unlimited. Traffic is not
// measured yet, so a plan carries no traffic limit (bandwidth stays 0).
// Yeni plan ölçülü sınırlarla başlar; 0 sınırsızdır. Trafik ölçülmediği için
// planda trafik sınırı yoktur.
const emptyPlan: ServicePlan = {
    id: 0,
    name: '',
    max_domains: 5,
    max_databases: 10,
    max_email_accounts: 50,
    disk_quota_mb: 10240,
    bandwidth_quota_mb: 0,
};

const limitText = (n: number, unlimited: string) => (n > 0 ? String(n) : unlimited);

function PlansTab({ isAdmin }: { isAdmin: boolean }) {
    const { t } = useI18n();
    const plans = useRemote(PLANS_URL, decodePlans);
    const [editing, setEditing] = useState<ServicePlan | null>(null);
    const [saving, setSaving] = useState(false);
    const load = () => void plans.retry();
    // A plan is added, edited or removed only on the list as the server last
    // gave it.
    // Plan yalnız sunucunun son verdiği liste üzerinde eklenir, düzenlenir ya
    // da kaldırılır.
    const plansKnown = plans.remote.state === 'known' && !plans.reading;
    const send = async (url: string, init: RequestInit): Promise<Response | null> => {
        setSaving(true);
        try {
            return await fetch(url, init);
        } catch {
            showToast('error', t('common.resultUnknown'));
            load();
            return null;
        } finally {
            setSaving(false);
        }
    };

    const save = async () => {
        if (!editing) return;
        const isNew = editing.id === 0;
        const res = await send(isNew ? '/api/v1/plans' : `/api/v1/plans/${editing.id}`, {
            method: isNew ? 'POST' : 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(editing),
        });
        if (!res) return;
        if (!res.ok) {
            showToast('error', apiErrorText(await readApiError(res), t));
            return;
        }
        showToast('success', isNew ? t('plans.created') : t('plans.updated'));
        setEditing(null);
        load();
    };

    const remove = async (p: ServicePlan) => {
        if (!confirm(`${p.name}?`)) return;
        const res = await send(`/api/v1/plans/${p.id}`, { method: 'DELETE' });
        if (!res) return;
        if (!res.ok) {
            showToast('error', apiErrorText(await readApiError(res), t));
            return;
        }
        showToast('success', t('plans.deleted'));
        load();
    };

    const unlimited = t('plans.unlimited');
    const fmtGB = (mb: number) => (mb <= 0 ? unlimited : mb >= 1024 ? `${(mb / 1024).toFixed(0)} GB` : `${mb} MB`);

    return (
        <div>
            {/* 0 = unlimited; traffic is not measured yet (alpha.114). */}
            <p className="mb-3 text-xs text-fg-subtle">{t('plans.hint')}</p>
            <div className="mb-3 flex items-center justify-between">
                <span className="text-xs text-fg-subtle">{(countText(mapRemote(plans.remote, (rows) => rows.length)) === '1' ? t('common.itemsTotalOne') : t('common.itemsTotal', { n: countText(mapRemote(plans.remote, (rows) => rows.length)) }))}</span>
                <Button variant="primary" icon={Plus} disabled={!plansKnown} onClick={() => setEditing({ ...emptyPlan })}>
                    {t('plans.add')}
                </Button>
            </div>

            {editing && (
                <div className="mb-4 rounded-xl border border-border bg-surface-2/50 p-4">
                    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
                        <label className="col-span-2 sm:col-span-3 lg:col-span-1">
                            <span className="mb-1 block text-xs text-fg-muted">{t('plans.form.name')}</span>
                            <input value={editing.name} onChange={(e) => setEditing({ ...editing, name: e.target.value })} className={inputClass} autoFocus />
                        </label>
                        <NumField label={t('plans.col.domains')} value={editing.max_domains} onChange={(v) => setEditing({ ...editing, max_domains: v })} />
                        <NumField label={t('plans.col.databases')} value={editing.max_databases} onChange={(v) => setEditing({ ...editing, max_databases: v })} />
                        <NumField label={t('plans.col.mail')} value={editing.max_email_accounts} onChange={(v) => setEditing({ ...editing, max_email_accounts: v })} />
                        <NumField label={`${t('plans.col.disk')} (MB)`} value={editing.disk_quota_mb} onChange={(v) => setEditing({ ...editing, disk_quota_mb: v })} />
                    </div>
                    <div className="mt-3 flex justify-end gap-2">
                        <Button icon={X} onClick={() => setEditing(null)}>{t('users.cancel')}</Button>
                        <Button variant="primary" icon={Save} onClick={save} disabled={saving || !plansKnown || !editing.name.trim()}>
                            {t('plans.save')}
                        </Button>
                    </div>
                </div>
            )}

            <RemoteGate
                remote={plans.remote}
                checking={t('plans.checking')}
                failed={t('plans.unknown')}
                onRetry={() => void plans.retry()}
                busy={plans.reading}
                className="py-3"
            >
                {(shown) => (shown.value.length === 0 ? (
                    <KnownEmpty of={shown} icon={Layers} title={t('plans.empty')} hint={t('plans.emptyHint')} />
                ) : (
                    <div className="overflow-x-auto rounded-xl border border-border-strong bg-surface">
                        <table className="w-full text-sm">
                            <thead>
                                <tr className="border-b border-border text-left text-xs font-semibold text-fg-muted">
                                    <th className="px-4 py-2.5">{t('plans.col.name')}</th>
                                    <th className="px-4 py-2.5">{t('plans.col.domains')}</th>
                                    <th className="px-4 py-2.5">{t('plans.col.databases')}</th>
                                    <th className="px-4 py-2.5">{t('plans.col.mail')}</th>
                                    <th className="px-4 py-2.5">{t('plans.col.disk')}</th>
                                    {isAdmin && <th className="px-4 py-2.5">{t('plans.col.owner')}</th>}
                                    <th className="px-4 py-2.5">{t('plans.col.subscribers')}</th>
                                    <th className="px-4 py-2.5" />
                                </tr>
                            </thead>
                            <tbody>
                                {shown.value.map((p) => (
                                    <tr key={p.id} className="border-b border-border last:border-0 hover:bg-surface-2/60">
                                        <td className="px-4 py-3 text-base font-medium text-fg">{p.name}</td>
                                        <td className="px-4 py-3 text-fg-muted">{limitText(p.max_domains, unlimited)}</td>
                                        <td className="px-4 py-3 text-fg-muted">{limitText(p.max_databases, unlimited)}</td>
                                        <td className="px-4 py-3 text-fg-muted">{limitText(p.max_email_accounts, unlimited)}</td>
                                        <td className="px-4 py-3 text-fg-muted">{fmtGB(p.disk_quota_mb)}</td>
                                        {isAdmin && <td className="px-4 py-3 text-fg-muted">{p.owner || t('plans.ownerGlobal')}</td>}
                                        <td className="px-4 py-3 text-fg-muted">{p.subscribers ?? 0}</td>
                                        <td className="px-4 py-3">
                                            {p.mine !== false && <div className="flex items-center justify-end gap-0.5">
                                                <RowBtn disabled={!plansKnown} title={t('plans.edit')} onClick={() => setEditing({ ...p })}>
                                                    <Pencil className="h-4 w-4" />
                                                </RowBtn>
                                                <RowBtn danger disabled={!plansKnown || saving} title={t('users.delete')} onClick={() => remove(p)}>
                                                    <Trash2 className="h-4 w-4" />
                                                </RowBtn>
                                            </div>}
                                        </td>
                                    </tr>
                ))}
                        </tbody>
                    </table>
                </div>
            ))}
            </RemoteGate>
        </div>
    );
}

function Tab({ active, onClick, label }: { active: boolean; onClick: () => void; label: string }) {
    return (
        <button
            onClick={onClick}
            className={`-mb-px border-b-2 px-3 py-2.5 text-sm font-medium transition-colors ${
                active ? 'border-primary text-primary' : 'border-transparent text-fg-muted hover:text-fg'
            }`}
        >
            {label}
        </button>
    );
}

function RowBtn({ children, title, onClick, danger, disabled }: { children: React.ReactNode; title: string; onClick: () => void; danger?: boolean; disabled?: boolean }) {
    return (
        <button
            type="button"
            title={title}
            aria-label={title}
            onClick={onClick}
            disabled={disabled}
            className={`rounded-md p-1.5 text-fg-subtle transition-colors hover:bg-surface-2 disabled:pointer-events-none disabled:opacity-40 ${danger ? 'hover:text-danger' : 'hover:text-fg'}`}
        >
            {children}
        </button>
    );
}

function NumField({ label, value, onChange }: { label: string; value: number; onChange: (v: number) => void }) {
    return (
        <label>
            <span className="mb-1 block text-xs text-fg-muted">{label}</span>
            <input type="number" value={value} onChange={(e) => onChange(parseInt(e.target.value) || 0)} className={inputClass} />
        </label>
    );
}
