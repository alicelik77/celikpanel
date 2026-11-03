import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { inputClass } from './ui';
import {
    PERMISSION_IDS, grantable, groupSoftware, inUseText, itemLabel, overLimitText, quantity, softwareGroupName, softwareName, toggle,
    type Numbers, type RightsPreview, type RightsSelection, type RightsView,
} from '../lib/rights';

// The three parts of a rights set, edited the same way everywhere (D-042 4):
// quantities, permissions (which jobs) and software (which component and
// version). Only what the person editing may hand out is selectable (P2); an
// item the set already holds but the editor may not give again stays visible,
// ticked and fixed, with the reason beside it. 0 is unlimited and says so.
//
// Hak kümesinin üç parçası her yerde aynı biçimde düzenlenir: sayılar, izinler
// ve yazılımlar. Yalnız düzenleyenin verebileceği seçilebilir; kümede olup
// yeniden verilemeyen öğe görünür, işaretli ve sabit kalır.

export function RightsSetEditor({ grantor, selection, onSelection, numbers, onNumbers, reseller = false, grantorIsReseller = false, idPrefix }: {
    /** What the person editing may hand out (GET /api/v1/rights/catalog). */
    grantor: RightsView;
    selection: RightsSelection;
    onSelection: (next: RightsSelection) => void;
    numbers: Numbers;
    onNumbers: (next: Numbers) => void;
    /** A reseller's set: the pool, with the number of customers. */
    reseller?: boolean;
    /** The person editing is a reseller (the reason an item is fixed differs). */
    grantorIsReseller?: boolean;
    idPrefix: string;
}) {
    const { t } = useI18n();
    const fixedReason = t(grantorIsReseller ? 'rights.notYours' : 'rights.notOffered');
    const software = groupSoftware([
        ...grantor.software,
        ...selection.software.filter((id) => !grantor.software.some((item) => item.id === id)).map((id) => ({ id, granted: false })),
    ]);
    const qty = (key: keyof Numbers, label: string, unit?: string) => (
        <label key={key} className="min-w-0">
            <span className="mb-1 block text-xs text-fg-muted">{unit ? `${label} (${unit})` : label}</span>
            <input
                type="number"
                min={0}
                inputMode="numeric"
                value={numbers[key]}
                onChange={(e) => onNumbers({ ...numbers, [key]: quantity(e.target.value) })}
                className={inputClass}
                data-rights-quantity={key}
            />
            <span className="mt-0.5 block text-xs text-fg-subtle">{numbers[key] > 0 ? ' ' : t('rights.unlimitedShort')}</span>
        </label>
    );
    return (
        <div className="grid gap-4" data-rights-editor={idPrefix}>
            <fieldset className="min-w-0">
                <legend className="text-sm font-semibold text-fg">{t(reseller ? 'rights.part.pool' : 'rights.part.quantities')}</legend>
                <p className="mt-0.5 text-xs text-fg-subtle">{t(reseller ? 'rights.part.poolHint' : 'rights.part.quantitiesHint')}</p>
                <div className="mt-2 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
                    {reseller && qty('max_customers', t('rights.qty.customers'))}
                    {qty('max_domains', t('rights.qty.domains'))}
                    {qty('max_databases', t('rights.qty.databases'))}
                    {qty('max_email_accounts', t('rights.qty.mailboxes'))}
                    {qty('disk_quota_mb', t('rights.qty.disk'), 'MB')}
                    {qty('traffic_quota_mb', t('rights.qty.traffic'), 'MB')}
                </div>
                <p className="mt-1 text-xs text-fg-subtle" data-rights-traffic-hint="">{t(reseller ? 'rights.qty.trafficHintPool' : 'rights.qty.trafficHint')}</p>
            </fieldset>

            <fieldset className="min-w-0">
                <legend className="text-sm font-semibold text-fg">{t('rights.part.permissions')}</legend>
                <p className="mt-0.5 text-xs text-fg-subtle">{t('rights.part.permissionsHint')}</p>
                <ul className="mt-2 grid gap-x-4 gap-y-1 sm:grid-cols-2">
                    {PERMISSION_IDS.map((id) => {
                        const open = grantable(grantor, 'permissions', id);
                        const checked = selection.permissions.includes(id);
                        return (
                            <li key={id}>
                                <label className={`flex items-start gap-2.5 py-1 max-sm:min-h-11 ${open ? 'cursor-pointer' : 'cursor-not-allowed'}`} data-rights-permission={id}>
                                    <input
                                        type="checkbox"
                                        className="mt-1 h-4 w-4 shrink-0 accent-primary"
                                        checked={checked}
                                        disabled={!open}
                                        onChange={(e) => onSelection({ ...selection, permissions: toggle(selection.permissions, id, e.target.checked) })}
                                    />
                                    <span className="min-w-0 text-sm">
                                        <span className={open ? 'text-fg' : 'text-fg-muted'}>{t(`rights.perm.${id}` as TranslationKey)}</span>
                                        {!open && <span className="block text-xs text-fg-subtle">{fixedReason}</span>}
                                    </span>
                                </label>
                            </li>
                        );
                    })}
                </ul>
            </fieldset>

            <fieldset className="min-w-0">
                <legend className="text-sm font-semibold text-fg">{t('rights.part.software')}</legend>
                <p className="mt-0.5 text-xs text-fg-subtle">{t('rights.part.softwareHint')}</p>
                {grantor.installed_unknown && <p className="mt-1 text-xs text-warning">{t('rights.installedUnknown')}</p>}
                {software.length === 0 ? (
                    <p className="mt-2 text-sm text-fg-muted">{t('rights.softwareNone')}</p>
                ) : (
                    <div className="mt-2 grid gap-3 sm:grid-cols-2">
                        {software.map(({ group, items }) => {
                            const heading = softwareGroupName(group);
                            return (
                                <div key={group} className="min-w-0 rounded-lg border border-border p-3" data-rights-software-group={group}>
                                    <p className="text-sm font-medium text-fg">{heading.product ?? t(heading.key === 'acme' ? 'rights.group.acme' : heading.key === 'app' ? 'rights.group.app' : 'rights.group.extension')}</p>
                                    <ul className="mt-1 flex flex-wrap gap-x-4 gap-y-1">
                                        {items.map((item) => {
                                            const open = grantable(grantor, 'software', item.id);
                                            return (
                                                <li key={item.id}>
                                                    <label className={`flex items-center gap-2 py-1 text-sm max-sm:min-h-11 ${open ? 'cursor-pointer text-fg' : 'cursor-not-allowed text-fg-muted'}`} title={open ? undefined : fixedReason} data-rights-software={item.id}>
                                                        <input
                                                            type="checkbox"
                                                            className="h-4 w-4 shrink-0 accent-primary"
                                                            checked={selection.software.includes(item.id)}
                                                            disabled={!open}
                                                            onChange={(e) => onSelection({ ...selection, software: toggle(selection.software, item.id, e.target.checked) })}
                                                        />
                                                        <span>{softwareName(item.id)}</span>
                                                    </label>
                                                </li>
                                            );
                                        })}
                                    </ul>
                                </div>
                            );
                        })}
                    </div>
                )}
            </fieldset>
        </div>
    );
}

/**
 * What a change will do, before it is made (P7): the sentence for the move,
 * what is added and taken away, what keeps running, and what is above a new
 * limit. `sentence` is the screen's own words for this change.
 */
export function RightsPreviewBox({ preview, sentence, id }: { preview: RightsPreview; sentence: string; id: string }) {
    const { t } = useI18n();
    const list = (items: string[]) => items.map((item) => itemLabel(item, t)).join(', ');
    return (
        <section id={id} tabIndex={-1} role="alert" className="rounded-lg border border-warning-mark/50 bg-warning-mark/15 p-4 text-sm leading-6 text-fg focus:outline-none" data-rights-preview={preview.leaves_plan ? 'leaves-plan' : 'change'}>
            <p className="max-w-[75ch] font-semibold">{sentence}</p>
            {preview.subscriptions !== undefined && preview.subscriptions > 0 && (
                <p className="mt-1">{t('rights.preview.accounts', { n: preview.subscriptions })}</p>
            )}
            {preview.added.length > 0 && <p className="mt-2"><span className="font-semibold">{t('rights.preview.added')}</span> {list(preview.added)}</p>}
            {preview.removed.length > 0 && <p className="mt-2"><span className="font-semibold">{t('rights.preview.removed')}</span> {list(preview.removed)}</p>}
            {preview.in_use.length > 0 && (
                <div className="mt-2">
                    <p className="font-semibold">{t('rights.preview.inUse')}</p>
                    <ul className="mt-1 list-disc pl-5">{preview.in_use.map((line) => <li key={line.item}>{inUseText(line, t)}</li>)}</ul>
                </div>
            )}
            {preview.over_limit.length > 0 && (
                <div className="mt-2">
                    <p className="font-semibold">{t('rights.preview.over')}</p>
                    <ul className="mt-1 list-disc pl-5">{preview.over_limit.map((line) => <li key={line}>{overLimitText(line, t)}</li>)}</ul>
                </div>
            )}
            {preview.added.length === 0 && preview.removed.length === 0 && preview.in_use.length === 0 && preview.over_limit.length === 0 && (
                <p className="mt-2 text-fg-muted">{t('rights.preview.nothingElse')}</p>
            )}
            {preview.installed_unknown && <p className="mt-2 text-xs text-fg-muted">{t('rights.installedUnknown')}</p>}
        </section>
    );
}
