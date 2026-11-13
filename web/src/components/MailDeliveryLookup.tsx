import { useState } from 'react';
import { Search } from 'lucide-react';
import { useMailOutgoingWords } from '../i18n/mailOutgoing';
import type { TranslationKey } from '../i18n/en';
import { useRemote } from '../lib/remote';
import { Button, RemoteGate, inputClass } from './ui';

// "Did my mail arrive?" (alpha.128, gap G9): what happened to this domain's
// mail in the last days, from Postfix's own log, in plain words. Only this
// domain's own messages are ever shown (the server filters by domain).
// Nothing is read until the person asks.
//
// "Postam ulaştı mı?": bu alan adının postasına ne olduğu, düz sözlerle.

interface Entry {
    at: string;
    from: string;
    to: string;
    status: 'sent' | 'deferred' | 'bounced' | 'expired' | 'refused' | 'queued' | string;
    relay?: string;
    reason?: string;
    direction: 'in' | 'out';
}

interface Lookup {
    days: number;
    result: { read: boolean; incomplete?: boolean; covered_since?: string; entries: Entry[] };
}

function decodeLookup(raw: unknown): Lookup {
    const value = raw as Partial<Lookup> | null;
    if (!value || !value.result || !Array.isArray(value.result.entries)) throw new Error('delivery');
    return value as Lookup;
}

export function MailDeliveryLookup({ domainId }: { domainId: number }) {
    const { t, locale } = useMailOutgoingWords();
    const [address, setAddress] = useState('');
    const [asked, setAsked] = useState<string | null>(null);
    const read = useRemote(asked, decodeLookup);
    const time = (iso: string) => new Date(iso).toLocaleString(locale, { dateStyle: 'short', timeStyle: 'short' });
    const ask = () => {
        const query = new URLSearchParams({ days: '7' });
        if (address.trim()) query.set('address', address.trim());
        setAsked(`/api/v1/domains/${domainId}/mail/delivery?${query.toString()}&n=${Date.now()}`);
    };
    return (
        <section className="mt-5 rounded-xl border border-border bg-surface p-4" data-mail-delivery>
            <h3 className="mb-1 text-sm font-semibold text-fg">{t('mdl.title')}</h3>
            <p className="mb-3 max-w-[75ch] text-sm text-fg-muted">{t('mdl.desc')}</p>
            <form className="flex flex-wrap items-end gap-2" onSubmit={(event) => { event.preventDefault(); ask(); }}>
                <input type="email" value={address} onChange={(event) => setAddress(event.target.value)} placeholder={t('mdl.placeholder')}
                    aria-label={t('mdl.placeholder')} className={`${inputClass} field-lg`} />
                <Button type="submit" icon={Search} loading={read.reading}>{t('mdl.search')}</Button>
            </form>
            {asked && (
                <div className="mt-3">
                    <RemoteGate remote={read.remote} checking={t('mdl.reading')} failed={t('mdl.failed')} onRetry={() => void read.retry()} busy={read.reading}>
                        {({ value }) => value.result.entries.length === 0 ? (
                            <p className="text-sm text-fg-muted">{t('mdl.none', { days: value.days })}</p>
                        ) : (
                            <ul className="divide-y divide-border text-sm">
                                {value.result.entries.map((entry, index) => (
                                    <li key={`${entry.at}-${entry.to}-${index}`} className="py-2">
                                        <p className="flex flex-wrap items-baseline justify-between gap-2">
                                            <span className="min-w-0 break-words text-fg">{entry.from} → {entry.to}</span>
                                            <span className="text-xs text-fg-muted">{time(entry.at)}</span>
                                        </p>
                                        <p className={`text-xs ${entry.status === 'sent' ? 'text-success' : entry.status === 'deferred' ? 'text-warning' : 'text-danger'}`}>
                                            {t(`mdl.status.${entry.status}` as TranslationKey)}
                                            {entry.relay && entry.status === 'sent' ? ` (${entry.relay === 'mailbox' ? t('mdl.toMailbox') : entry.relay})` : ''}
                                        </p>
                                        {entry.reason && entry.status !== 'sent' && <p className="break-words text-xs text-fg-subtle">{entry.reason}</p>}
                                    </li>
                                ))}
                            </ul>
                        )}
                    </RemoteGate>
                </div>
            )}
        </section>
    );
}

export default MailDeliveryLookup;
