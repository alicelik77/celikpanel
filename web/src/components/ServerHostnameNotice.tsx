// This server's own host name, when public DNS does not answer for it
// (alpha.121, Frankfurt 4 Nov). Read-only and cached on the server: the page
// only reads; "Check again" is a read; the record is added only after the
// owner reviewed its exact text and pressed Add, through the zone's own
// record endpoint, once per press.
// Sunucunun kendi adı genel DNS'te çözülmüyorsa söylenir; kayıt yalnız
// gözden geçirilip onaylanınca, bir kez eklenir.
import { useRef, useState } from 'react';
import { useI18n } from '../i18n';
import { apiErrorText, readApiError } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { Link } from '../router';
import { decodeServerHostnameDNS, hostnameDomain, SERVER_HOSTNAME_DNS_URL, serverHostnameRecordText, type ServerHostnameDNS } from '../lib/serverHostname';
import { Button, Dialog, KeepWords } from './ui';
import { LifeLine } from './LifeLine';

const JSON_HEADERS = { 'Content-Type': 'application/json' };

export function ServerHostnameNotice() {
    const { t } = useI18n();
    const [recheck, setRecheck] = useState(0);
    const url = recheck > 0 ? `${SERVER_HOSTNAME_DNS_URL}?recheck=1&n=${recheck}` : SERVER_HOSTNAME_DNS_URL;
    const handle = useRemote(url, decodeServerHostnameDNS);
    const [confirming, setConfirming] = useState(false);
    const [adding, setAdding] = useState(false);
    const [notice, setNotice] = useState<{ tone: 'ok' | 'bad' | 'info'; text: string } | null>(null);
    const inFlight = useRef(false);
    // A read that failed, or a name that resolves, shows nothing here.
    const value = handle.remote.state === 'known' ? handle.remote.value : null;
    if (!value || (value.state !== 'missing' && value.state !== 'other_address')) {
        return notice ? <div className="mb-6"><LifeLine tone={notice.tone} attr="server-hostname-notice"><KeepWords text={notice.text} /></LifeLine></div> : null;
    }

    const add = async (check: ServerHostnameDNS) => {
        const zone = check.zone;
        if (!zone || inFlight.current) return;
        inFlight.current = true;
        setAdding(true);
        let said: { tone: 'ok' | 'bad' | 'info'; text: string } | null = null;
        try {
            for (const record of zone.missing) {
                let res: Response | null = null;
                try {
                    res = await fetch(zone.recordsURL, {
                        method: 'POST',
                        headers: JSON_HEADERS,
                        body: JSON.stringify({ name: record.name + '.', type: record.type, content: record.content, ttl: record.ttl, prio: 0 }),
                    });
                } catch {
                    res = null;
                }
                if (res === null) {
                    // The answer was lost: the record may have been added. It is
                    // never sent again by itself; Check again reads DNS.
                    said = { tone: 'info', text: t('hostDNS.addLost', { record: serverHostnameRecordText(record) }) };
                    break;
                }
                if (!res.ok) {
                    said = { tone: 'bad', text: t('hostDNS.addFailed', { record: serverHostnameRecordText(record), zone: zone.name, reason: apiErrorText(await readApiError(res), t) }) };
                    break;
                }
            }
            if (!said) said = { tone: 'ok', text: t('hostDNS.added', { zone: zone.name, host: check.hostname }) };
        } finally {
            inFlight.current = false;
            setAdding(false);
            setConfirming(false);
            setNotice(said);
        }
        invalidate('dns');
        // One read, as pressed; never a loop.
        setRecheck((n) => n + 1);
    };

    const zone = value.state === 'missing' ? value.zone : null;
    const host = value.hostname;
    let next: string;
    if (value.state === 'other_address') {
        next = t('hostDNS.otherNext', { addresses: value.addresses.join(', '), ip: value.ipv4 || value.ipv6 });
    } else if (zone && zone.missing.length > 0) {
        next = t('hostDNS.zoneNext', { zone: zone.name });
    } else if (zone) {
        next = t('hostDNS.presentNext', { zone: zone.name, records: zone.present.join(', ') });
    } else {
        next = t('hostDNS.externalNext', { domain: hostnameDomain(host), host, ip: value.ipv4 || value.ipv6 });
    }
    return (
        <section className="mb-6 rounded-lg border border-warning-mark/50 bg-warning-mark/20 p-4 text-sm leading-6 text-fg" role="status" data-server-hostname-dns={value.state}>
            <p className="font-medium"><KeepWords text={t(value.state === 'missing' ? 'hostDNS.missing' : 'hostDNS.other', { host })} /></p>
            <p className="mt-1"><KeepWords text={next} /></p>
            <p className="mt-1 text-fg-muted">{t('hostDNS.why')}</p>
            <div className="mt-3 flex flex-wrap items-center gap-2">
                {zone && zone.missing.length > 0 && (
                    <Button variant="primary" className="max-sm:min-h-11" onClick={() => setConfirming(true)} disabled={adding} data-server-hostname-add="">
                        {t('hostDNS.add')}
                    </Button>
                )}
                <Button className="max-sm:min-h-11" onClick={() => setRecheck((n) => n + 1)} loading={handle.reading} disabled={adding}>
                    {t('hostDNS.recheck')}
                </Button>
                {zone && <Link to={zone.editorPath} className="text-primary hover:underline">{t('hostDNS.openZone', { zone: zone.name })}</Link>}
            </div>
            {notice && <div className="mt-3"><LifeLine tone={notice.tone} attr="server-hostname-notice"><KeepWords text={notice.text} /></LifeLine></div>}
            {confirming && zone && (
                <Dialog
                    id="server-hostname-records"
                    title={t('hostDNS.dialogTitle', { zone: zone.name })}
                    busy={adding}
                    width="lg"
                    onDismiss={() => setConfirming(false)}
                    actions={
                        <>
                            <Button onClick={() => setConfirming(false)} disabled={adding}>{t('common.cancel')}</Button>
                            <Button variant="primary" onClick={() => void add(value)} loading={adding} disabled={adding} data-server-hostname-confirm="">
                                {t('hostDNS.confirm')}
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm leading-6 text-fg">{t('hostDNS.dialogBody', { zone: zone.name, host })}</p>
                    <pre className="mt-2 overflow-x-auto whitespace-pre-wrap break-all rounded-lg border border-border bg-surface-2 p-3 font-mono text-xs text-fg">{zone.missing.map(serverHostnameRecordText).join('\n')}</pre>
                    <p className="mt-2 text-sm leading-6 text-fg-muted">{t('hostDNS.dialogWait')}</p>
                </Dialog>
            )}
        </section>
    );
}
