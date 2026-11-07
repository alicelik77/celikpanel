import { useRemote } from '../lib/remote';
import { CouldNotCheck } from './ui';
import { useDataToolsWords } from './dataToolsWords';

// The server's IP addresses (alpha.128, G14), administrators only, read only.
// Every site answers on every address, so each is shared by all sites;
// assigning a dedicated address is not offered yet (its design note says why).
//
// Sunucunun IP adresleri; salt okunur.

interface Address {
    address: string;
    prefix: number;
    iface: string;
    family: string;
    isPrivate: boolean;
}

function decode(raw: unknown): { addresses: Address[]; sites: number } {
    const v = (raw ?? {}) as Record<string, unknown>;
    const list = Array.isArray(v.addresses) ? v.addresses : [];
    return {
        sites: typeof v.sites === 'number' ? v.sites : 0,
        addresses: list.filter((a): a is Record<string, unknown> => !!a && typeof a === 'object' && typeof (a as Record<string, unknown>).address === 'string')
            .map((a) => ({
                address: String(a.address), prefix: typeof a.prefix === 'number' ? a.prefix : 0,
                iface: typeof a.interface === 'string' ? a.interface : '', family: a.family === 'ipv6' ? 'IPv6' : 'IPv4', isPrivate: a.private === true,
            })),
    };
}

export function IPAddressesCard() {
    const { t } = useDataToolsWords();
    const list = useRemote('/api/v1/network/ip-addresses', decode);
    return (
        <section className="mt-5 space-y-2 rounded-lg border border-border bg-surface p-4">
            <h3 className="text-sm font-semibold text-fg">{t('ipl.title')}</h3>
            <p className="text-xs text-fg-muted">{t('ipl.hint')}</p>
            {list.remote.state === 'loading' && <p className="text-sm text-fg-muted">{t('ipl.reading')}</p>}
            {list.remote.state === 'unknown' && <CouldNotCheck text={t('ipl.unread')} onRetry={() => void list.retry()} busy={list.reading} />}
            {list.remote.state === 'known' && (
                <ul className="divide-y divide-border text-sm">
                    {list.remote.value.addresses.map((a) => (
                        <li key={a.address} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-1.5">
                            <span className="font-mono text-fg">{a.address}/{a.prefix}</span>
                            <span className="text-xs text-fg-muted">{a.family} · {a.iface}</span>
                            <span className="text-xs text-fg-muted">{a.isPrivate ? t('ipl.private') : t('ipl.public')}</span>
                            <span className="text-xs text-fg-muted">{t('ipl.shared', { n: list.remote.state === 'known' ? list.remote.value.sites : 0 })}</span>
                        </li>
                    ))}
                </ul>
            )}
        </section>
    );
}
