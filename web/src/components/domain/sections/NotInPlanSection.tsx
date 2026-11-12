// A section the plan does not include (alpha.132, drawing X08). Before, such a
// tab was left out without a word; now its page says in one block what the
// section is, that the plan does not include it, whom to ask and what to ask
// for. Who that is depends on the viewer: a customer asks their provider (a
// reseller) or the server administrator, named as My plan names them (GET
// /api/v1/my/subscription); a reseller adds it to a customer's plan
// themselves, or asks the administrator for their own; a team member's
// account owner asks theirs. Nothing here changes anything.
//
// Planın içermediği bölüm: ne olduğu, planda olmadığı, kime ve ne için
// başvurulacağı tek blokta. Kişi, Planım'ın adlandırdığı gibi adlandırılır.
import { Lock, Users } from 'lucide-react';
import { useNavigate } from '../../../router';
import { useI18n } from '../../../i18n';
import type { TranslationKey } from '../../../i18n/en';
import { lastKnown, useRemote } from '../../../lib/remote';
import { MY_SUBSCRIPTION_URL, decodeMyPlan, type MyPlanContact } from '../../../lib/myPlan';
import { domainSectionIcons } from '../../../lib/domainSectionIcons';
import { Button } from '../../ui';
import { SECTION_LABELS, type SectionId } from '../sections';

type Closable = 'mail' | 'databases' | 'backups' | 'files';
const ABOUT: Record<Closable, TranslationKey> = {
    mail: 'domain.notInPlan.about.mail',
    databases: 'domain.notInPlan.about.databases',
    backups: 'domain.notInPlan.about.backups',
    files: 'domain.notInPlan.about.files',
};

function named(contact: MyPlanContact | undefined): string {
    if (!contact) return '';
    return [contact.name, contact.email ? `(${contact.email})` : ''].filter(Boolean).join(' ');
}

export function NotInPlanSection({ section, rights, role }: { section: SectionId; rights: readonly string[]; role: string }) {
    const { t } = useI18n();
    const navigate = useNavigate();
    const reseller = role === 'reseller';
    const teamMember = role === 'additional_user';
    // My plan's own read: whom this account asks. A team member has no plan
    // of its own and is not given that read.
    const plan = useRemote(teamMember ? null : MY_SUBSCRIPTION_URL, decodeMyPlan);
    const contact = lastKnown(plan.remote)?.value.contact;
    const who = named(contact);
    const Icon = domainSectionIcons[section];
    const sectionName = t(SECTION_LABELS[section]);
    const right = rights.map((item) => t(`rights.perm.${item}` as TranslationKey)).join(', ');

    const title = t(reseller ? 'domain.notInPlan.titleDomain' : teamMember ? 'domain.notInPlan.titleAccount' : 'domain.notInPlan.title', { section: sectionName });
    const whoText = teamMember
        ? t('domain.notInPlan.who.team')
        : reseller
          ? t(who ? 'domain.notInPlan.who.resellerNamed' : 'domain.notInPlan.who.reseller', { who })
          : contact?.kind === 'reseller' && who
            ? t('domain.notInPlan.who.provider', { who })
            : contact?.kind === 'administrator' && who
              ? t('domain.notInPlan.who.administrator', { who })
              : t('domain.notInPlan.who.providerUnnamed');
    const askText = t(reseller ? 'domain.notInPlan.ask.reseller' : teamMember ? 'domain.notInPlan.ask.team' : 'domain.notInPlan.ask.own', { right });
    const about = (section in ABOUT) ? t(ABOUT[section as Closable]) : '';

    return (
        <section className="rounded-xl border border-border bg-surface p-5" aria-labelledby="domain-not-in-plan-title" data-not-in-plan-page={section}>
            <h2 id="domain-not-in-plan-title" className="flex items-center gap-2 text-base font-semibold text-fg">
                <Lock className="h-4 w-4 shrink-0 text-fg-muted" aria-hidden="true" />
                {title}
            </h2>
            <div className="mt-4 flex gap-4">
                <Icon className="hidden h-8 w-8 shrink-0 text-fg-muted sm:block" aria-hidden="true" />
                <div className="min-w-0 flex-1 text-sm leading-6 text-fg">
                    {about && <p>{about}</p>}
                    <dl className="mt-3 grid gap-x-6 gap-y-1.5 sm:grid-cols-[minmax(0,11rem)_minmax(0,1fr)]">
                        <dt className="text-fg-muted">{t('domain.notInPlan.whoLabel')}</dt>
                        <dd data-not-in-plan-who="">{whoText}</dd>
                        <dt className="text-fg-muted">{t(reseller ? 'domain.notInPlan.addLabel' : 'domain.notInPlan.askLabel')}</dt>
                        <dd>{askText}</dd>
                        <dt className="text-fg-muted">{t('domain.notInPlan.thenLabel')}</dt>
                        <dd>{t('domain.notInPlan.then')}</dd>
                    </dl>
                    {reseller && (
                        <div className="mt-4">
                            <Button type="button" icon={Users} onClick={() => navigate('/users')}>{t('domain.notInPlan.openUsers')}</Button>
                        </div>
                    )}
                </div>
            </div>
        </section>
    );
}
