import { useState } from 'react';
import { useI18n } from '../i18n';
import type { SuspendedAccount } from '../lib/api';
import { BrandMark } from './BrandMark';
import { LanguageSwitcher } from './LanguageSwitcher';
import { ThemeSwitcher } from './ThemeSwitcher';
import { Button, Spinner } from './ui';

// The only screen the owner of a suspended subscription sees (2 Nov 2026). It
// replaces every page: nothing here starts anything on the server. It says why
// (the reason, when it was given, and since when), who acts (the reseller or the
// administrator), what to do (contact them) and how it resumes (when they resume
// the subscription, signing in again opens the panel). D-024.
//
// Askıya alınmış aboneliğin sahibinin gördüğü tek ekran. Her sayfanın yerini
// alır; sunucuda hiçbir şey başlatmaz. Nedeni, kimin işlem yapacağını (bayi ya
// da yönetici), ne yapılacağını (onlarla iletişim) ve nasıl devam edileceğini
// (abonelik yeniden açılınca yeniden giriş) söyler.
export function SuspendedScreen({ username, suspended, onSignedOut }: {
    username: string;
    suspended: SuspendedAccount;
    onSignedOut: () => void;
}) {
    const { t, locale, screensReady } = useI18n();
    const [leaving, setLeaving] = useState(false);
    const [failed, setFailed] = useState(false);

    async function signOut() {
        setLeaving(true);
        setFailed(false);
        try {
            const response = await fetch('/api/v1/auth/logout', { method: 'POST' });
            if (!response.ok && response.status !== 401) throw new Error('logout');
            onSignedOut();
        } catch {
            setFailed(true);
        } finally {
            setLeaving(false);
        }
    }

    // The words are screen copy: until they have arrived nothing is drawn as a key.
    if (!screensReady) {
        return <div className="flex min-h-screen items-center justify-center bg-bg"><Spinner /></div>;
    }

    const { contact } = suspended;
    const since = suspended.since ? new Date(suspended.since) : null;
    const sinceText = since && !Number.isNaN(since.getTime()) ? since.toLocaleDateString(locale, { year: 'numeric', month: 'long', day: 'numeric' }) : '';
    // "Contact <name> (<email>)": the person who can resume it.
    const who = contact.kind === 'reseller' ? t('suspended.contact.reseller') : t('suspended.contact.administrator');
    const named = [contact.name, contact.email ? `(${contact.email})` : ''].filter(Boolean).join(' ');

    return (
        <div className="min-h-screen bg-bg text-fg">
            <header className="border-b border-border bg-surface px-4 py-5 sm:px-8">
                <div className="mx-auto flex max-w-3xl flex-wrap items-center justify-between gap-4">
                    <div className="flex items-center gap-3 font-semibold"><BrandMark className="h-7 w-7 text-primary" />CelikPanel</div>
                    <div className="flex items-center gap-2">
                        <LanguageSwitcher />
                        <ThemeSwitcher />
                        <Button variant="secondary" disabled={leaving} onClick={() => void signOut()}>{t('user.logout')}</Button>
                    </div>
                </div>
            </header>
            <main className="mx-auto max-w-3xl px-4 py-10 sm:px-8 sm:py-16">
                <p className="mb-5 break-words text-sm text-fg-muted">{username}</p>
                {failed && <p role="alert" className="mb-4 text-danger">{t('suspended.signOutFailed')}</p>}
                <section aria-labelledby="suspended-heading" className="space-y-5" data-suspended-screen={suspended.kind}>
                    <h1 id="suspended-heading" className="text-2xl font-semibold">
                        {t(suspended.kind === 'reseller' ? 'suspended.titleReseller' : 'suspended.title')}
                    </h1>
                    <p className="max-w-prose">{t(suspended.kind === 'reseller' ? 'suspended.helpReseller' : 'suspended.help')}</p>
                    <dl className="max-w-prose space-y-4 rounded-lg border border-border bg-surface p-4 text-sm leading-6">
                        <div>
                            <dt className="font-semibold">{t('suspended.why')}</dt>
                            <dd className="break-words text-fg-muted">
                                {suspended.reason ?? t('suspended.noReason')}
                                {sinceText && <> {t('suspended.since', { date: sinceText })}</>}
                            </dd>
                        </div>
                        <div>
                            <dt className="font-semibold">{t('suspended.whoActs')}</dt>
                            <dd className="break-words text-fg-muted">
                                {contact.kind === 'none' ? t('suspended.contact.none') : `${who}${named ? `: ${named}` : ''}`}
                            </dd>
                        </div>
                        <div>
                            <dt className="font-semibold">{t('suspended.next')}</dt>
                            <dd className="break-words text-fg-muted">
                                {contact.kind === 'none' ? t('suspended.next.none') : t('suspended.next.contact', { who: contact.name ?? who })}
                                {contact.email && <> <a className="font-medium text-primary underline" href={`mailto:${contact.email}`}>{contact.email}</a></>}
                            </dd>
                        </div>
                        <div>
                            <dt className="font-semibold">{t('suspended.resumes')}</dt>
                            <dd className="text-fg-muted">{t('suspended.resumes.text')}</dd>
                        </div>
                    </dl>
                    <p className="max-w-prose text-sm text-fg-muted">{t('suspended.nothingLost')}</p>
                </section>
            </main>
        </div>
    );
}
