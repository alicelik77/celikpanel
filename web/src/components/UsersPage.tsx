import { useEffect, useRef, useState } from 'react';
import { Ban, Copy, Layers, LogIn, Pause, Pencil, Play, Plus, Power, Send, Trash2, Users, XCircle } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { useAuth } from '../auth/AuthContext';
import { type PanelUser, type ServicePlan } from '../lib/api';
import { Button, CouldNotCheck, ErrorBanner, KnownEmpty, RemoteGate, StatusDot, inputClass } from './ui';
import { PageHeader } from './PageHeader';
import { readApiError, refusalText, refusalToast, type ApiError } from '../lib/apiError';
import { NEW_PLAN, UNLIMITED_CHOICE, assignablePlans, editedFromCustomerPlan, newAccountRightsBody, newAccountRightsMissing, rightsSummary, sendRights, stateName, type EditedPlan, type NewAccountRights } from '../lib/rights';
import { AccountRightsPanel, NewAccountRightsSection } from './AccountRights';
import { BuiltinPlanRow, OfferedItemsTab, PlanEditorGate, ResellerPlansTab } from './PlanRights';
import { AdminRightsNotices } from './RightsNotices';
import { useNavigate } from '../router';
import { countText, invalidate, mapRemote, useRemote } from '../lib/remote';
import { PLANS_URL, USERS_URL, decodePlans, decodeUsers } from '../lib/accounts';
import { TeamMembersPage } from './TeamMembersPage';
import { InvitedStatus, PendingInvitations, useInvitationActions, useInvitationOutcome } from './Invitations';
import { SuspensionDialog, type SuspensionTarget } from './SuspensionDialog';
import { DeletableAccounts } from './DeletableAccounts';
import { useSuspendedOwners } from '../lib/suspension';
import { TransferRequestsList, TransferRowAction } from './SubscriptionTransfer';
import { confirmAction } from '../lib/confirmAction';
import { PasswordInput } from './PasswordInput';
import { passwordAcceptable } from '../lib/passwordPolicy';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { confirmLeaveIfDirty } from './kit/save/leaveGuard';

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

type UsersTab = 'accounts' | 'plans' | 'resellerPlans' | 'offered' | 'deletable';

function AccountUsersPage({ role }: { role: 'admin' | 'reseller' }) {
    const { t } = useI18n();
    const isAdmin = role === 'admin';
    const [tab, setTab] = useState<UsersTab>('accounts');
    // alpha.132: a tab with unsaved changes (a plan form, the Offered tab)
    // asks before it is left, as leaving the page does.
    const go = (next: UsersTab) => {
        if (next !== tab) void confirmLeaveIfDirty().then((leave) => { if (leave) setTab(next); });
    };

    return (
        <div className="p-6 md:p-8">
            <PageHeader
                title={t('nav.users')}
                subtitle={t('users.subtitle')}
                breadcrumb={[t('common.home'), t('nav.users')]}
            />

            <div className="mb-4 flex items-center gap-1 border-b border-border">
                <Tab active={tab === 'accounts'} onClick={() => go('accounts')} label={t('users.tab.accounts')} />
                {/* Resellers have their own plans too (alpha.114). / Bayilerin de kendi planları var. */}
                <Tab active={tab === 'plans'} onClick={() => go('plans')} label={t('users.tab.plans')} />
                {/* alpha.117 (D-042): the administrator's reseller plans and what the server offers. */}
                {isAdmin && <Tab active={tab === 'resellerPlans'} onClick={() => go('resellerPlans')} label={t('rights.tab.resellerPlans')} />}
                {isAdmin && <Tab active={tab === 'offered'} onClick={() => go('offered')} label={t('rights.tab.offered')} />}
                {/* Suspended subscriptions, with what they still hold (2 Nov 2026). / Askıdaki abonelikler. */}
                <Tab active={tab === 'deletable'} onClick={() => go('deletable')} label={t('susp.del.tab')} />
            </div>

            {tab === 'accounts' && <AccountsTab isAdmin={isAdmin} onCreatePlan={(reseller) => go(reseller ? 'resellerPlans' : 'plans')} />}
            {tab === 'plans' && <PlansTab isAdmin={isAdmin} />}
            {tab === 'resellerPlans' && isAdmin && <ResellerPlansTab />}
            {tab === 'offered' && isAdmin && <OfferedItemsTab />}
            {tab === 'deletable' && <DeletableAccounts />}
        </div>
    );
}

function AccountsTab({ isAdmin, onCreatePlan }: { isAdmin: boolean; onCreatePlan: (reseller: boolean) => void }) {
    const { t, locale } = useI18n();
    const users = useRemote(USERS_URL, decodeUsers);
    const plans = useRemote(PLANS_URL, decodePlans);
    const invitation = useInvitationOutcome();
    // Resend or cancel an invitation from its account's row (alpha.114).
    const rowInvite = useInvitationActions();
    const [showForm, setShowForm] = useState(false);
    // Invite: no password; the person chooses theirs by a link (alpha.114).
    const [invite, setInvite] = useState(false);

    const [username, setUsername] = useState('');
    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [newRole, setNewRole] = useState('customer');
    // The Rights section (alpha.117, D-042 6.1): Unlimited is chosen, by name.
    const [newRights, setNewRights] = useState<NewAccountRights>(UNLIMITED_CHOICE);
    const [saving, setSaving] = useState(false);
    // One row's rights panel is open at a time.
    // Aynı anda tek satırın hak paneli açıktır.
    const [panel, setPanel] = useState<{ user: PanelUser } | null>(null);
    // The dialogue that suspends or resumes every service of one account, and the
    // accounts that own a suspended subscription (marked in the list).
    // Bir hesabın tüm hizmetlerini askıya alan / açan pencere ve askıda
    // aboneliği olan hesaplar (listede işaretlenir).
    const [suspension, setSuspension] = useState<SuspensionTarget | null>(null);
    const suspended = useSuspendedOwners();

    // Every change re-reads this list and every other place that shows
    // accounts, invitations, plans or their usage: no page refresh is needed.
    // Her değişiklik bu listeyi ve hesapları, davetleri, planları gösteren her
    // yeri yeniden okutur; sayfa yenilemesi gerekmez.
    const load = () => {
        invalidate('users', 'plans', 'rights');
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
    // The plans a new customer may be given: global ones, and a reseller's own.
    const customerPlanOptions = assignablePlans(planOptions, {}, isAdmin);
    // What still keeps Create off, in the form's own words.
    const createMissing = [
        ...(!username.trim() ? [t('users.form.username')] : []),
        ...(!email.trim() ? [t('users.form.email')] : []),
        ...(!invite && !passwordAcceptable(password, 'account') ? [t('users.create.passwordRule')] : []),
        ...newAccountRightsMissing(newRights, t),
    ];

    // Conflict answers (quota, children, duplicates) carry real reasons from
    // the API; the coded contract picks a localized text when the refusal
    // has a code, else the server message — never a vague generic.
    // Çakışma yanıtları (kota, alt hesap, mükerrer) API'den gerçek nedenlerle
    // gelir; kodlu sözleşme, ret kodluysa yerelleştirilmiş metni, değilse
    // sunucu mesajını seçer — asla belirsiz bir genel değil.
    // The refusal stays in the toast until it is closed, with its fix; the
    // create form also keeps it beside its button (owner feedback 2026-10-31).
    const navigate = useNavigate();

    // ?rights=<account> / ?resume=<account> (alpha.127): a plan, pool or
    // suspension notice elsewhere (a customer's Access tab) opens that
    // account's rights panel, or its suspend/resume window, here. Used once
    // and removed from the address; an account this list does not hold opens
    // nothing.
    // Başka ekrandaki bildirim, hesabın hak panelini ya da sürdürme penceresini açar.
    const accountLinkRef = useRef(false);
    useEffect(() => {
        if (accountLinkRef.current || users.remote.state !== 'known' || typeof window === 'undefined') return;
        const params = new URLSearchParams(window.location.search);
        const rights = params.get('rights');
        const resume = params.get('resume');
        if (rights === null && resume === null) return;
        accountLinkRef.current = true;
        const found = users.remote.value.find((u) => u.username === (rights ?? resume));
        if (found && rights !== null) setPanel({ user: found });
        else if (found) setSuspension({ kind: 'user', id: found.id, name: found.username });
        params.delete('rights');
        params.delete('resume');
        const query = params.toString();
        navigate(`${window.location.pathname}${query ? `?${query}` : ''}${window.location.hash}`, { replace: true });
    }, [users.remote, navigate]);
    const [createRefusal, setCreateRefusal] = useState<ApiError | null>(null);
    const apiError = async (res: Response, sentence: 'users.createFailed' | 'users.changeFailed' = 'users.changeFailed') => {
        const problem = await readApiError(res);
        showToast('error', refusalText(problem, t, sentence), refusalToast(problem, t, navigate));
        return problem;
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
        setCreateRefusal(null);
        try {
            const body: Record<string, unknown> = invite
                ? { username, email, role: newRole, invite: true, lang: locale }
                : { username, email, password, role: newRole };
            body.rights = newAccountRightsBody(newRights, newRole);
            const res = await send('/api/v1/users', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            });
            if (!res) return;
            if (!res.ok) {
                // Said once, in the banner beside the form (useRefusal): no notice.
                setCreateRefusal(await readApiError(res));
                return;
            }
            showToast('success', t('users.created'));
            if (invite) {
                // The account exists; what happened to its invitation is the
                // answer's to say, and an unreadable answer says it is not known.
                try {
                    invitation.report(((await res.json()) as { invitation?: unknown }).invitation);
                } catch {
                    showToast('error', t('common.resultUnknown'));
                }
            }
            setShowForm(false);
            setUsername('');
            setEmail('');
            setPassword('');
            setInvite(false);
            setNewRights(UNLIMITED_CHOICE);
            load();
        } finally {
            setSaving(false);
        }
    };

    const setStatus = async (u: PanelUser, status: 'active' | 'suspended') => {
        const proceed = async () => {
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
        if (!(status === 'suspended')) {
            await proceed();
            return;
        }
        await confirmAction({ message: t('users.suspendConfirm', { name: u.username }), tone: 'danger', action: proceed });
    };

    const deleteUser = async (u: PanelUser) => {
        await confirmAction({ message: t('users.deleteConfirm', { name: u.username }), confirmLabel: t('common.delete'), tone: 'danger',
            action: async (report) => {
                // The server's own step (alpha.127).
                const res = await sendWithSteps(report, flowSteps(t, 'userDelete', ['delete'], { name: u.username }), t, `userDelete.${u.id}`,
                    (headers) => send(`/api/v1/users/${u.id}`, { method: 'DELETE', headers }));
                if (!res) return;
                if (!res.ok) {
                    await apiError(res);
                    return;
                }
                showToast('success', t('users.deleted'));
                load();
            },
        });
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

            {/* After the update: accounts that became Unlimited, and pools nearly used up. */}
            {isAdmin && (
                <AdminRightsNotices
                    onOpenAccount={(name) => {
                        const found = users.remote.state === 'known' ? users.remote.value.find((u) => u.username === name) : undefined;
                        if (found) setPanel({ user: found });
                    }}
                    onSetRights={(userId) => {
                        const found = users.remote.state === 'known' ? users.remote.value.find((u) => u.id === userId) : undefined;
                        if (found) setPanel({ user: found });
                    }}
                />
            )}

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
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
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
                            <PasswordInput field="account" value={invite ? '' : password} disabled={invite} onChange={setPassword} maxLength={1024} />
                        </label>
                        <label>
                            <span className="mb-1 block text-xs text-fg-muted">{t('users.form.role')}</span>
                            <select
                                value={newRole}
                                onChange={(e) => { setNewRole(e.target.value); setNewRights(UNLIMITED_CHOICE); }}
                                className={inputClass}
                                disabled={!isAdmin}
                            >
                                <option value="customer">{t('users.role.customer')}</option>
                                {isAdmin && <option value="reseller">{t('users.role.reseller')}</option>}
                            </select>
                        </label>
                    </div>
                    {/* Rights: always visible; Unlimited, a plan, or Custom (D-042 6.1). */}
                    <NewAccountRightsSection
                        role={newRole}
                        choice={newRights}
                        onChoice={setNewRights}
                        customerPlans={customerPlanOptions}
                        grantorIsReseller={!isAdmin}
                        onCreatePlan={() => onCreatePlan(newRole === 'reseller')}
                    />
                    <label className="mt-3 flex items-start gap-2 text-sm text-fg" data-invite-option="">
                        <input type="checkbox" className="mt-1" checked={invite} onChange={(e) => setInvite(e.target.checked)} />
                        <span>
                            {t('acct.invite.option')}
                            <span className="block text-xs text-fg-muted">{t('acct.invite.optionHint')}</span>
                        </span>
                    </label>
                    <ErrorBanner className="mt-3" error={createRefusal} sentence="users.createFailed" />
                    <div className="mt-3 flex flex-wrap items-center justify-end gap-2">
                        {/* Why Create is off, beside it (owner feedback 2026-10-31). */}
                        {createMissing.length > 0 && <p id="users-create-reason" className="mr-auto text-sm text-fg" data-blocked-reason="">{t('users.create.missing', { fields: createMissing.join(', ') })}</p>}
                        <Button onClick={() => setShowForm(false)}>{t('users.cancel')}</Button>
                        <Button
                            variant="primary"
                            icon={Plus}
                            onClick={createUser}
                            aria-describedby={createMissing.length > 0 ? 'users-create-reason' : undefined}
                            disabled={saving || !usersKnown || createMissing.length > 0}
                        >
                            {t('users.create')}
                        </Button>
                    </div>
                </div>
            )}

            {panel && (
                <AccountRightsPanel
                    key={panel.user.id}
                    user={panel.user}
                    isAdmin={isAdmin}
                    onClose={() => setPanel(null)}
                    onCreatePlan={() => onCreatePlan(panel.user.role === 'reseller')}
                />
            )}

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
                                    <th className="px-4 py-2.5">{t('users.col.rights')}</th>
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
                                                {u.account_type === 'additional_user' && u.parent_name ? t('users.role.additionalOf', { owner: u.parent_name }) : t(roleKey(u.role))}
                                            </span>
                                        </td>
                                        <td className="px-4 py-3">
                                            {/* Invited, not signed in yet: not shown as active (alpha.114). */}
                                            {u.status === 'active' && u.invitation ? (
                                                <InvitedStatus invitation={u.invitation} cancelledHint={t('acct.invite.status.cancelledUsers')} />
                                            ) : (
                                                <span className="inline-flex items-center gap-1.5 text-fg-muted">
                                                    <StatusDot ok={u.status === 'active'} />
                                                    {u.status === 'active' ? t('users.status.active') : t('users.status.suspended')}
                                                </span>
                                            )}
                                            {/* Services suspended (not the sign-in block above). / Hizmetler askıda. */}
                                            {suspended.owners.has(u.id) && (
                                                <span className="mt-1 block w-fit rounded-md bg-warning/15 px-2 py-0.5 text-xs font-medium text-warning" data-services-suspended={u.id}>
                                                    {t('susp.badge')}
                                                </span>
                                            )}
                                        </td>
                                        {isAdmin && <td className="px-4 py-3 text-fg-muted">{u.parent_name || '—'}</td>}
                                        <td className="px-4 py-3 text-fg-muted">
                                            {u.subscriptions} / {u.domains}
                                        </td>
                                        {/* Unlimited, Plan: X or Custom (D-042 P3). / Haklar. */}
                                        <td className="px-4 py-3 text-fg-muted" data-rights-state={u.rights?.state ?? ''}>
                                            {u.rights ? accountRightsText(u.rights, u.role === 'reseller', t) : '—'}
                                        </td>
                                        <td className="px-4 py-3">
                                            {u.role !== 'admin' && (
                                                <div className="flex items-center justify-end gap-0.5">
                                                    {u.status === 'active' && u.invitation?.id !== undefined && (
                                                        <InvitationRowActions
                                                            id={u.invitation.id}
                                                            username={u.username}
                                                            disabled={!usersKnown || rowInvite.busy !== null}
                                                            act={rowInvite.act}
                                                        />
                                                    )}
                                                    {u.account_type !== 'additional_user' && (u.role === 'customer' || (isAdmin && u.role === 'reseller')) && (
                                                        <RowBtn disabled={!usersKnown} title={t('rights.edit')} onClick={() => setPanel({ user: u })}>
                                                            <Layers className="h-4 w-4" />
                                                        </RowBtn>
                                                    )}
                                                    <TransferRowAction user={u} disabled={!usersKnown} />
                                                    <RowBtn disabled={!usersKnown} title={t('users.loginAs')} onClick={() => impersonate(u)}>
                                                        <LogIn className="h-4 w-4" />
                                                    </RowBtn>
                                                    {/* Every service of the account: sites, mail, databases, VPN,
                                                        cron and file access (2 Nov 2026). The dialogue reads what
                                                        would change and says it before anything is sent. */}
                                                    {u.account_type !== 'additional_user' && (
                                                        <RowBtn
                                                            disabled={!usersKnown}
                                                            title={t(suspended.owners.has(u.id) ? 'susp.row.resume' : suspended.known ? 'susp.row.suspend' : 'susp.row.either')}
                                                            onClick={() => setSuspension({ kind: 'user', id: u.id, name: u.username })}
                                                        >
                                                            {suspended.owners.has(u.id) ? <Power className="h-4 w-4" /> : <Ban className="h-4 w-4" />}
                                                        </RowBtn>
                                                    )}
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
            <PendingInvitations />
            {suspension && <SuspensionDialog target={suspension} onClose={() => setSuspension(null)} />}
            <TransferRequestsList />
            {invitation.dialog}
            {rowInvite.dialog}
        </div>
    );
}

const limitText = (n: number, unlimited: string) => (n > 0 ? String(n) : unlimited);

// The Rights column: the state's name; "mixed" for a customer whose
// subscriptions differ.
function accountRightsText(rights: { state: string; plan_name?: string }, reseller: boolean, t: (key: TranslationKey, vars?: Record<string, string | number>) => string): string {
    if (rights.state === 'mixed') return t('users.rights.mixed');
    return stateName(rights.state, rights.plan_name, t, reseller);
}

function PlansTab({ isAdmin }: { isAdmin: boolean }) {
    const { t } = useI18n();
    const plans = useRemote(PLANS_URL, decodePlans);
    const [editing, setEditing] = useState<EditedPlan | null>(null);
    const [saving, setSaving] = useState(false);
    // A plan is added, edited or removed only on the list as the server last
    // gave it.
    // Plan yalnız sunucunun son verdiği liste üzerinde eklenir, düzenlenir ya
    // da kaldırılır.
    const plansKnown = plans.remote.state === 'known' && !plans.reading;

    const remove = async (p: ServicePlan) => {
        await confirmAction({
            message: t('plans.deleteConfirm', { name: p.name }),
            confirmLabel: t('common.delete'),
            tone: 'danger',
            action: async () => {
                setSaving(true);
                try {
                    const result = await sendRights(`/api/v1/plans/${p.id}`, 'DELETE');
                    if (result.kind === 'lost') showToast('error', t('common.resultUnknown'));
                    else if (result.kind === 'refused') showToast('error', refusalText(result.error, t, 'plans.deleteFailed'), refusalToast(result.error, t));
                    else showToast('success', t('plans.deleted'));
                    // Plans show in the account list and in the plan usage too: all re-read.
                    invalidate('plans', 'rights');
                } finally {
                    setSaving(false);
                }
            },
        });
    };

    const unlimited = t('plans.unlimited');
    const fmtGB = (mb: number) => (mb <= 0 ? unlimited : mb >= 1024 ? `${(mb / 1024).toFixed(0)} GB` : `${mb} MB`);

    return (
        <div>
            {/* 0 = unlimited; traffic is measured monthly, web only (alpha.118). */}
            <p className="mb-3 text-xs text-fg-subtle">{t('plans.hint')}</p>
            {/* The built-in Unlimited plan: a fixed row (D-042 P4). */}
            <BuiltinPlanRow kind="customer" ownField={!isAdmin} />
            <div className="mb-3 flex items-center justify-between">
                <span className="text-xs text-fg-subtle">{(countText(mapRemote(plans.remote, (rows) => rows.length)) === '1' ? t('common.itemsTotalOne') : t('common.itemsTotal', { n: countText(mapRemote(plans.remote, (rows) => rows.length)) }))}</span>
                {/* While a plan form is open its Save is the action; Add plan
                    comes back when the form closes (owner, 6 Nov 2026).
                    Plan formu açıkken eylem formun Kaydet'idir. */}
                {editing === null && (
                    <Button variant="primary" icon={Plus} disabled={!plansKnown} onClick={() => setEditing({ ...NEW_PLAN })} data-plan-add="">
                        {t('plans.add')}
                    </Button>
                )}
            </div>

            {editing && <PlanEditorGate kind="customer" initial={editing} grantorIsReseller={!isAdmin} onClose={() => setEditing(null)} />}

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
                                    <th className="px-4 py-2.5">{t('rights.qty.traffic')}</th>
                                    <th className="px-4 py-2.5">{t('rights.col.rights')}</th>
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
                                        <td className="px-4 py-3 text-fg-muted">{fmtGB(p.bandwidth_quota_mb ?? 0)}</td>
                                        <td className="px-4 py-3 text-fg-muted">{rightsSummary(p.rights, t)}</td>
                                        {isAdmin && <td className="px-4 py-3 text-fg-muted">{p.owner || t('plans.ownerGlobal')}</td>}
                                        <td className="px-4 py-3 text-fg-muted">{p.subscribers ?? 0}</td>
                                        <td className="px-4 py-3">
                                            <div className="flex items-center justify-end gap-0.5">
                                                {p.mine !== false && <RowBtn disabled={!plansKnown || editing !== null} title={t('plans.edit')} onClick={() => setEditing(editedFromCustomerPlan(p))}>
                                                    <Pencil className="h-4 w-4" />
                                                </RowBtn>}
                                                {/* alpha.128 (G13): a new plan filled in from this one; accounts stay. */}
                                                <RowBtn disabled={!plansKnown || editing !== null} title={t('limits.plan.copy')} onClick={() => setEditing({ ...editedFromCustomerPlan(p), id: 0, name: t('limits.plan.copyName', { name: p.name }) })}>
                                                    <Copy className="h-4 w-4" />
                                                </RowBtn>
                                                {p.mine !== false && <RowBtn danger disabled={!plansKnown || saving} title={t('users.delete')} onClick={() => remove(p)}>
                                                    <Trash2 className="h-4 w-4" />
                                                </RowBtn>}
                                            </div>
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

// Resend or cancel the open invitation of an account, from its row.
// Satırdan açık daveti yeniden gönder ya da iptal et.
function InvitationRowActions({ id, username, disabled, act }: {
    id: number;
    username: string;
    disabled: boolean;
    act: (id: number, username: string, action: 'email' | 'revoke') => Promise<void>;
}) {
    const { t } = useI18n();
    return (
        <>
            <RowBtn disabled={disabled} title={t('acct.invite.rowResend')} onClick={() => void act(id, username, 'email')}>
                <Send className="h-4 w-4" />
            </RowBtn>
            <RowBtn disabled={disabled} title={t('acct.invite.revoke')} onClick={() => void act(id, username, 'revoke')}>
                <XCircle className="h-4 w-4" />
            </RowBtn>
        </>
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
