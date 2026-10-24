// Which account a Node.js application runs as (v0.1.0-alpha.107, D-038 §6).
// A new application runs as its site's own user; one made before runs as the
// shared web user and says so, with the action that moves it: its files that
// the web user owns go to the site's user, the unit is rewritten and the
// application restarted, and it must answer; otherwise everything is put
// back. Asked first, with the interruption; nothing moves by itself.
//
// Bir Node.js uygulamasının hangi hesapla çalıştığı; eski uygulama yalnız
// istenince, kanıtla ve geri almayla taşınır.
import { useState } from 'react';
import { useI18n } from '../i18n';
import { Button, Dialog, KeepWords } from './ui';
import { useRemote } from '../lib/remote';
import { LifeLine, type Tone } from './LifeLine';

interface AppAccount {
    runs_as: 'site' | 'web';
    site_user: string;
    web_user: string;
}

function decodeAppAccount(raw: unknown): AppAccount {
    const body = raw as Partial<AppAccount> | null;
    if (!body || (body.runs_as !== 'site' && body.runs_as !== 'web') || typeof body.site_user !== 'string' || typeof body.web_user !== 'string') throw new Error('shape');
    return body as AppAccount;
}

export function AppAccountLine({ domainId }: { domainId: number }) {
    const { t } = useI18n();
    const url = `/api/v1/domains/${domainId}/app/account`;
    const handle = useRemote(url, decodeAppAccount);
    const [confirming, setConfirming] = useState(false);
    const [moving, setMoving] = useState(false);
    const [result, setResult] = useState<{ tone: Tone; text: string } | null>(null);
    if (handle.remote.state !== 'known') return null;
    const account = handle.remote.value;
    const move = async () => {
        setMoving(true);
        setResult(null);
        try {
            const res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
            const raw = await res.text();
            let body: { error?: string; rolled_back?: boolean };
            try {
                body = JSON.parse(raw);
            } catch (error) {
                body = { error: raw.slice(0, 200) || String(error) };
            }
            setResult(res.ok
                ? { tone: 'ok', text: t('life.app.moved', { user: account.site_user }) }
                : { tone: body.rolled_back ? 'warn' : 'bad', text: t(body.rolled_back ? 'life.app.rolledBack' : 'life.app.failed', { reason: body.error ?? '' }) });
        } catch {
            setResult({ tone: 'info', text: t('life.app.unknown') });
        } finally {
            setMoving(false);
            setConfirming(false);
            void handle.retry();
        }
    };
    return (
        <div className="mt-3 space-y-2" data-app-account={account.runs_as}>
            {account.runs_as === 'site' ? (
                <LifeLine tone="ok" attr="app-account-site">{t('life.app.site', { user: account.site_user })}</LifeLine>
            ) : (
                <>
                    <LifeLine tone="warn" attr="app-account-web"><KeepWords text={t('life.app.web', { user: account.web_user })} /></LifeLine>
                    <div className="pl-6">
                        <Button onClick={() => setConfirming(true)} disabled={moving} className="max-sm:min-h-11" data-app-account-offer="">
                            {t('life.app.move', { user: account.site_user })}
                        </Button>
                    </div>
                </>
            )}
            {moving && <LifeLine tone="info" attr="app-account-moving">{t('life.app.moving')}</LifeLine>}
            {result && <LifeLine tone={result.tone} attr="app-account-result"><KeepWords text={result.text} /></LifeLine>}
            {confirming && (
                <Dialog
                    id="app-account-move"
                    title={t('life.app.dialogTitle', { user: account.site_user })}
                    busy={moving}
                    onDismiss={() => setConfirming(false)}
                    actions={
                        <>
                            <Button onClick={() => setConfirming(false)} disabled={moving}>{t('common.cancel')}</Button>
                            <Button variant="primary" onClick={() => void move()} loading={moving} disabled={moving} data-app-account-confirm="">
                                {t('life.app.confirm')}
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm leading-6 text-fg"><KeepWords text={t('life.app.dialogBody', { user: account.site_user, web: account.web_user })} /></p>
                </Dialog>
            )}
        </div>
    );
}

// Where separation is shared today (D-038 §6), said plainly as a fact.
export function IsolationFact({ kind }: { kind: 'tools' | 'mail' }) {
    const { t } = useI18n();
    return <p className="text-sm leading-6 text-fg-subtle" data-isolation-fact={kind}>{t(kind === 'tools' ? 'life.isolation.tools' : 'life.isolation.mail')}</p>;
}
