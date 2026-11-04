import { ShieldAlert, ShieldCheck } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import type { RuntimeLimits, RuntimeRisk, RuntimeSandboxLevel } from '../lib/runtimeApps';

// The risk summary of uploaded programs, the same for the administrator who
// offers them and the customer who deploys one: what a program can do, what
// the sandbox blocks, how it reaches the network, what remains a risk; the
// limits; this server's sandbox level with its named gaps; and what is not
// covered at all (docs/RUNTIME-APPS.md).
//
// Yüklenen programların risk özeti; sunan yönetici ve kuran müşteri aynısını
// görür: yapabildikleri, korumalı alanın engelledikleri, ağ, kalan riskler,
// sınırlar, sunucunun koruma düzeyi ve kapsam dışı olanlar.

const sections: { field: keyof RuntimeRisk; title: TranslationKey; tone: string }[] = [
    { field: 'can_do', title: 'runtime.risk.canDo', tone: 'text-fg' },
    { field: 'blocked', title: 'runtime.risk.blocked', tone: 'text-success' },
    { field: 'network', title: 'runtime.risk.network', tone: 'text-fg' },
    { field: 'remains', title: 'runtime.risk.remains', tone: 'text-warning' },
];

export function RuntimeSandboxLine({ sandbox }: { sandbox: RuntimeSandboxLevel }) {
    const { t } = useI18n();
    if (!sandbox.known) return <p className="text-sm text-fg-muted">{t('runtime.sandbox.unknown')}</p>;
    const reduced = sandbox.level === 'reduced';
    return (
        <div className="space-y-1 text-sm">
            <p className="flex items-center gap-2 font-medium text-fg">
                {reduced ? <ShieldAlert className="h-4 w-4 text-warning" /> : <ShieldCheck className="h-4 w-4 text-success" />}
                {t(reduced ? 'runtime.sandbox.reduced' : 'runtime.sandbox.full')}
            </p>
            {reduced && (
                <ul className="list-disc space-y-0.5 pl-5 text-warning">
                    {sandbox.gaps.map((gap) => <li key={gap}>{t(('svcapps.sandbox.gap.' + gap) as TranslationKey)}</li>)}
                </ul>
            )}
        </div>
    );
}

export function RuntimeAppRisk({ risk, limits, sandbox }: { risk: RuntimeRisk; limits: RuntimeLimits; sandbox: RuntimeSandboxLevel }) {
    const { t } = useI18n();
    return (
        <div className="space-y-3 text-sm">
            <div className="grid gap-3 sm:grid-cols-2">
                {sections.map((section) => {
                    const codes = risk?.[section.field] ?? [];
                    if (codes.length === 0) return null;
                    return (
                        <div key={section.field}>
                            <div className={`mb-1 text-xs font-semibold uppercase tracking-wide ${section.tone}`}>{t(section.title)}</div>
                            <ul className="list-disc space-y-0.5 pl-5 text-fg-muted">
                                {codes.map((code) => <li key={code}>{t(('runtime.risk.' + code) as TranslationKey)}</li>)}
                            </ul>
                        </div>
                    );
                })}
            </div>
            <p className="text-xs text-fg-subtle">{t('runtime.limits', { memory: limits.memory_mb, cpu: limits.cpu_percent, tasks: limits.tasks_max })}</p>
            <RuntimeSandboxLine sandbox={sandbox} />
            <p className="text-xs text-warning">{t('runtime.notCovered')}</p>
        </div>
    );
}
