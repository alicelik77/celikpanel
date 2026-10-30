// CelikPanel's default HTTPS server on the Nginx page (alpha.113). Until now
// what CelikPanel did about it (set up, stepped back for the owner's own
// default_server, refused by nginx) was only in the Agent's journal.
//
// The card says the current reason first, then who acts, the concrete next
// action and what happens after it (D-024). A verified failure (the last
// attempt failed, nginx's own words shown), a step not taken yet (pending)
// and an unknown read are three different things. Reading never changes
// anything; "Set up now" / "Try again" is offered only where there is
// something to set up, and one press is one POST with a request identity.
//
// Nginx sayfasında CelikPanel'in varsayılan HTTPS sunucusu. Kart önce nedeni,
// sonra kimin yapacağını, ne yapılacağını ve sonra ne olacağını söyler.
// Okuma hiçbir şeyi değiştirmez; düğme yalnız kurulacak bir şey varsa çıkar.
import { useState } from 'react';
import { ShieldCheck } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError } from '../lib/apiError';
import { useLostAnswer } from '../lib/lostAnswer';
import { useRemote, type Remote } from '../lib/remote';
import {
    decodeDefaultHTTPS,
    defaultHTTPSOffersSetup,
    defaultHTTPSTone,
    NGINX_DEFAULT_HTTPS_URL,
    type DefaultHTTPSStatus,
} from '../lib/nginxDefaultHTTPS';
import { Button, ResultUnknown, RemoteGate, StatusDot } from './ui';

export function NginxDefaultHTTPSCard() {
    const { t } = useI18n();
    const handle = useRemote(NGINX_DEFAULT_HTTPS_URL, decodeDefaultHTTPS);
    return (
        <section className="mt-4 rounded-xl border border-border bg-surface" data-nginx-default-https="">
            <header className="flex items-center gap-2 border-b border-border px-5 py-3">
                <ShieldCheck className="h-4 w-4 text-fg-muted" aria-hidden="true" />
                <h2 className="text-sm font-semibold text-fg">{t('nginxdh.title')}</h2>
            </header>
            <div className="p-5">
                <RemoteGate remote={handle.remote} checking={t('nginxdh.checking')} failed={t('nginxdh.unknownRead')} onRetry={() => void handle.retry()} busy={handle.reading}>
                    {({ value }) => <DefaultHTTPSFacts status={value.status} reread={handle.retry} reading={handle.reading} />}
                </RemoteGate>
            </div>
        </section>
    );
}

const toneDot = { ok: 'success', info: 'idle', warn: 'danger', bad: 'danger', unknown: 'unknown' } as const;

function DefaultHTTPSFacts({ status, reread, reading }: { status: DefaultHTTPSStatus; reread: () => Promise<Remote<unknown>>; reading: boolean }) {
    const { t, locale } = useI18n();
    const [sending, setSending] = useState(false);
    const [refusal, setRefusal] = useState<string | null>(null);
    const answer = useLostAnswer(reread);
    const state = status.state;
    const key = (part: string) => `nginxdh.${state}.${part}` as TranslationKey;
    const vars = { file: status.ownerFile || status.path, version: status.nginxVersion };
    const reason = state === 'active'
        ? t(status.mode === 'placeholder' ? 'nginxdh.active.placeholder' : 'nginxdh.active.reject')
        : t(key('reason'), vars);
    const steps: [TranslationKey, string][] = [];
    if (state !== 'active' && state !== 'not_needed') {
        steps.push(['nginxdh.label.who', t(key('who'))], ['nginxdh.label.next', t(key('next'))]);
    } else {
        steps.push(['nginxdh.label.who', t('nginxdh.nobody')]);
    }
    if (state === 'not_needed' || state === 'pending' || state === 'failed' || state === 'unknown') {
        steps.push(['nginxdh.label.then', t(key('then'))]);
    }

    const setUp = async () => {
        if (sending || answer.holding) return;
        setSending(true);
        setRefusal(null);
        try {
            const res = await answer.send(NGINX_DEFAULT_HTTPS_URL, { method: 'POST', cache: 'no-store' });
            // No result of its own: the notice is up and the state is read again.
            if (!res) return;
            answer.settle();
            if (!res.ok) {
                setRefusal(apiErrorText(await readApiError(res), t));
                return;
            }
            await reread();
        } finally {
            setSending(false);
        }
    };

    const lastAt = status.lastAt ? new Date(status.lastAt) : null;
    return (
        <div className="space-y-3 text-sm">
            <ResultUnknown answer={answer} />
            <p className="flex items-center gap-2 font-medium text-fg" data-state={state}>
                <StatusDot tone={toneDot[defaultHTTPSTone(state)]} />
                {t(key('head'))}
            </p>
            <p className="text-fg-muted">{reason}</p>
            {status.detail && (state === 'failed' || state === 'unknown') && (
                <p className="break-words rounded-lg bg-surface-2 px-3 py-2 font-mono text-xs text-fg">{status.detail}</p>
            )}
            <dl className="space-y-2">
                {steps.map(([label, value]) => (
                    <div key={label} className="sm:flex sm:gap-3">
                        <dt className="shrink-0 text-fg-subtle sm:w-40">{t(label)}</dt>
                        <dd className="text-fg">{value}</dd>
                    </div>
                ))}
            </dl>
            {refusal && <p className="text-danger" role="alert">{refusal}</p>}
            <div className="flex flex-wrap items-center gap-2 pt-1">
                {defaultHTTPSOffersSetup(state) && (
                    <Button variant="primary" loading={sending} disabled={sending || answer.holding || reading} onClick={() => void setUp()}>
                        {t(sending ? 'nginxdh.running' : state === 'failed' ? 'nginxdh.retry' : 'nginxdh.setup')}
                    </Button>
                )}
                <Button loading={reading && !sending} disabled={sending || reading} onClick={() => void reread()}>
                    {t('nginxdh.check')}
                </Button>
            </div>
            <p className="text-xs text-fg-subtle">
                {status.nginxVersion ? t('nginxdh.version', { version: status.nginxVersion }) : t('nginxdh.versionUnknown')}
                {lastAt && !Number.isNaN(lastAt.getTime()) && (
                    <> · {t('nginxdh.lastAttempt', { time: lastAt.toLocaleString(locale) })}</>
                )}
            </p>
        </div>
    );
}
