import type { DemoAccount } from '../lib/api';
import type { TranslationKey } from '../i18n/en';

// The sign-in page's demo accounts (only a server started with --demo lists
// any). Its own chunk since alpha.134, fetched only when there is one to show,
// so the boot path does not carry it; its copy is screen copy (accountAccess).
// It imports nothing at run time that the boot path shares (the translator
// comes from the sign-in page; the role's initial stands in for an icon): a
// shared import here would split that code into a chunk of its own on the
// boot path.
// Demo hesapları; yalnız gösterilecek hesap varsa getirilir.

export function LoginDemoAccounts({ accounts, t, onPick }: { accounts: DemoAccount[]; t: (key: TranslationKey) => string; onPick: (account: DemoAccount) => void }) {
    if (accounts.length === 0) return null;
    return (
        <div className="mt-4 rounded-2xl border border-dashed border-border bg-surface/60 p-4">
            <div className="mb-1 flex items-center gap-2 text-sm font-semibold text-fg">
                <span className="inline-flex h-1.5 w-1.5 rounded-full bg-warning" />
                {t('acct.demo.title')}
            </div>
            <p className="mb-3 text-xs text-fg-muted">{t('acct.demo.hint')}</p>
            <div className="space-y-2">
                {accounts.map((acc) => (
                        <button
                            key={acc.username}
                            type="button"
                            onClick={() => onPick(acc)}
                            className="flex w-full items-center gap-3 rounded-lg border border-border bg-surface-2 px-3 py-2 text-left transition-colors hover:border-primary hover:bg-surface"
                        >
                            <span aria-hidden="true" className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-sm font-semibold uppercase text-primary">
                                {acc.role.slice(0, 1)}
                            </span>
                            <span className="min-w-0 flex-1">
                                <span className="block text-sm font-medium capitalize text-fg">{acc.role}</span>
                                <span className="block truncate font-mono text-xs text-fg-muted">
                                    {acc.username} · {acc.password}
                                </span>
                            </span>
                        </button>
                ))}
            </div>
        </div>
    );
}
