// The one confirmed webmail (Roundcube) update flow (alpha.107, D-038 §9; moved
// here in alpha.108 so the Components row and the webmail card start the SAME
// operation): the confirmation dialog, the visible waiting, and the result in
// the owner's words.
//
// alpha.109 (Frankfurt): the update is a durable server operation. Confirming
// sends one POST with a request id and follows the operation by reading it
// (polling never starts a second update; a lost answer is looked up by its
// request id). After a reload the screen re-attaches to a running update from
// the check's last_update, and afterwards it says the outcome: updated,
// failed and put back with the cause, not changed, or not known.
//
// Tek onaylı web postası güncelleme akışı. Güncelleme sunucuda kalıcı bir
// işlemdir: onay tek bir POST gönderir, işlem okunarak izlenir; yeniden
// yüklenince çalışan güncellemeye yeniden bağlanılır ve sonuç söylenir.
import { useEffect, useRef, useState } from 'react';
import { useI18n } from '../i18n';
import { Button, Dialog, KeepWords } from './ui';
import { LifeLine, type Tone } from './LifeLine';
import { readWebmailUpdateOperation as readOperation, ROUNDCUBE_UPDATE_URL, WEBMAIL_LINK_OFFERED, webmailUpdateOutcome, type WebmailUpdateOperation } from '../lib/roundcubeUpdate';

export interface RoundcubeUpdateOutcome {
    tone: Tone;
    text: string;
}

const POLL_MS = 1500;
function newRequestID(): string | null {
    try {
        const bytes = new Uint8Array(16);
        globalThis.crypto.getRandomValues(bytes);
        return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
    } catch {
        return null;
    }
}

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

export function useRoundcubeUpdate({ current, latest, lastUpdate, onChanged }: { current?: string; latest: string; lastUpdate?: WebmailUpdateOperation | null; onChanged: () => void }) {
    const { t } = useI18n();
    const [confirming, setConfirming] = useState(false);
    const [operation, setOperation] = useState<WebmailUpdateOperation | null>(null);
    const [following, setFollowing] = useState(false);
    const [notice, setNotice] = useState<RoundcubeUpdateOutcome | null>(null);
    const followed = useRef('');
    const alive = useRef(true);
    const changed = useRef(onChanged);
    changed.current = onChanged;
    useEffect(() => () => { alive.current = false; }, []);

    // follow reads the operation until it ends; it never sends a change.
    // follow, işlemi bitene dek okur; hiçbir değişiklik göndermez.
    const follow = async (id: string) => {
        if (followed.current === id) return;
        followed.current = id;
        setFollowing(true);
        let misses = 0;
        while (alive.current) {
            const next = await readOperation(`id=${encodeURIComponent(id)}`);
            if (!alive.current) return;
            if (next && next !== 'error') {
                misses = 0;
                setOperation(next);
                if (next.status === 'succeeded' || next.status === 'failed') break;
            } else if (++misses >= 40) {
                // The panel did not answer for a minute: the update goes on
                // on the server; a reload reads it again.
                setNotice({ tone: 'info', text: t('life.webmail.lostSight') });
                break;
            }
            await wait(POLL_MS);
        }
        if (!alive.current) return;
        followed.current = '';
        setFollowing(false);
        changed.current();
    };

    // Re-attach to an update that is still running (a reload, another tab).
    useEffect(() => {
        if (lastUpdate && (lastUpdate.status === 'queued' || lastUpdate.status === 'running')) void follow(lastUpdate.id);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [lastUpdate?.id, lastUpdate?.status]);

    const starting = useRef(false);
    const update = async () => {
        if (starting.current || following) return;
        starting.current = true;
        setNotice(null);
        const requestID = newRequestID();
        try {
            if (!requestID) {
                setNotice({ tone: 'bad', text: t('life.webmail.failed', { reason: t('life.webmail.noRequestID') }) });
                return;
            }
            let res: Response | null = null;
            try {
                res = await fetch(ROUNDCUBE_UPDATE_URL, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ version: latest, request_id: requestID }),
                });
            } catch {
                res = null;
            }
            if (res === null) {
                // The answer was lost: the request may have arrived. Look it up
                // by its id; never send it again.
                const found = await readOperation(`request_id=${requestID}`);
                if (found && found !== 'error') {
                    setOperation(found);
                    void follow(found.id);
                } else {
                    setNotice({ tone: 'info', text: t('life.webmail.answerLost') });
                }
                return;
            }
            const raw = await res.text();
            let body: { operation?: WebmailUpdateOperation; error?: string | { code?: string; message?: string }; code?: string } = {};
            try {
                body = JSON.parse(raw);
            } catch {
                body = { error: raw.slice(0, 200) };
            }
            if (res.status === 202 && body.operation) {
                setOperation(body.operation);
                void follow(body.operation.id);
                return;
            }
            const code = typeof body.error === 'object' ? body.error?.code : body.code;
            if (res.status === 409 && (code === 'service_operation_busy' || code === 'server_setup_busy')) {
                setNotice({ tone: 'info', text: t('life.webmail.busy') });
                return;
            }
            const reason = typeof body.error === 'object' ? body.error?.message ?? '' : body.error ?? '';
            setNotice({ tone: 'bad', text: t('life.webmail.failed', { reason }) });
        } finally {
            starting.current = false;
            setConfirming(false);
        }
    };

    // What the row and the card say: the operation followed here, otherwise
    // the last update the server read back, when it still matters.
    const shown = operation ?? lastUpdate ?? null;
    let result: RoundcubeUpdateOutcome | null = notice;
    let phase = '';
    const running = following || (shown !== null && (shown.status === 'queued' || shown.status === 'running'));
    if (shown && running) {
        phase = shown.phase === 'scanning' ? t('life.webmail.phaseScanning') : '';
    } else if (shown && !notice) {
        const said = webmailUpdateOutcome(shown);
        const ownTerminal = operation !== null;
        // A failure stays said until the server runs the release it aimed
        // at; a success is said where it was followed.
        const stillMatters = said.outcome !== 'updated' ? current !== shown.package_name : ownTerminal;
        if (stillMatters) result = outcomeText(t, said, current);
    }
    return { confirming, setConfirming, updating: running, phase, result, update };
}

type Translate = ReturnType<typeof useI18n>['t'];

function outcomeText(t: Translate, said: ReturnType<typeof webmailUpdateOutcome>, current?: string): RoundcubeUpdateOutcome {
    const vars = { from: said.from || current || '', to: said.to, backup: said.backup, reason: said.reason };
    switch (said.outcome) {
        case 'updated':
            return { tone: 'ok', text: said.backup ? t('life.webmail.done', vars) : t('life.webmail.doneNoBackup', vars) };
        case 'rolled_back':
            return said.previousAnswers === false
                ? { tone: 'bad', text: t('life.webmail.rolledBackDown', vars) }
                : { tone: 'warn', text: t('life.webmail.rolledBack', vars) };
        case 'rollback_failed':
            return { tone: 'bad', text: t('life.webmail.rollbackFailed', vars) };
        case 'interrupted':
            return { tone: 'warn', text: t('life.webmail.interrupted', vars) };
        case 'unknown':
            return { tone: 'info', text: t('life.webmail.outcomeUnknown', vars) };
        default:
            return said.code === 'CONFIG_OWNER_EDITED'
                ? { tone: 'warn', text: t('life.webmail.ownerEdited', vars) }
                : { tone: 'warn', text: t('life.webmail.notChanged', vars) };
    }
}

export function RoundcubeUpdateProgress({ updating, phase = '', result }: { updating: boolean; phase?: string; result: RoundcubeUpdateOutcome | null }) {
    const { t } = useI18n();
    return (
        <>
            {updating && (
                <LifeLine tone="info" attr="webmail-updating">
                    {t('life.webmail.updating')}
                    {phase ? ` ${phase}` : ''}
                </LifeLine>
            )}
            {!updating && result && <LifeLine tone={result.tone} attr="webmail-result"><KeepWords text={result.text} /></LifeLine>}
        </>
    );
}

export function RoundcubeUpdateDialog({ latest, updating, onCancel, onConfirm }: { latest: string; updating: boolean; onCancel: () => void; onConfirm: () => void }) {
    const { t } = useI18n();
    return (
        <Dialog
            id="webmail-update"
            title={t('life.webmail.dialogTitle', { latest })}
            busy={updating}
            width="lg"
            onDismiss={onCancel}
            actions={
                <>
                    <Button onClick={onCancel} disabled={updating}>{t('common.cancel')}</Button>
                    <Button variant="primary" onClick={onConfirm} loading={updating} disabled={updating} data-webmail-update-confirm="">
                        {t('life.webmail.confirm')}
                    </Button>
                </>
            }
        >
            <p className="text-sm leading-6 text-fg">{t('life.webmail.dialogBody', { latest })}</p>
            <p className="mt-2 text-sm leading-6 text-fg-muted">{t('life.webmail.dialogLeave')}</p>
        </Dialog>
    );
}

// OpenWebmailLink (alpha.109, owner: "I don't know how to open webmail"):
// the panel serves Roundcube at its own /webmail/ address; the link is
// offered only while webmail answers there.
// Web postası panelin /webmail/ adresindedir; bağlantı yalnız orada yanıt
// verirken sunulur.
//
// Held back (alpha.109 review): webmail is served on the panel's own origin,
// where the panel's script policy (which protects the administrator's
// session) keeps Roundcube's interface from starting. The link is offered
// once webmail has its own address (the next step); until then this flag
// stays false and nothing is drawn.
// Web postası kendi adresine kavuşana dek bağlantı gösterilmez.
export { WEBMAIL_LINK_OFFERED } from '../lib/roundcubeUpdate';

export function OpenWebmailLink({ ready, className = '' }: { ready: boolean | undefined; className?: string }) {
    const { t } = useI18n();
    if (!WEBMAIL_LINK_OFFERED || !ready) return null;
    const origin = typeof window !== 'undefined' && window.location ? window.location.origin : '';
    const url = `${origin}/webmail/`;
    return (
        <div className={`flex flex-wrap items-center gap-x-3 gap-y-1.5 ${className}`} data-open-webmail="">
            <a
                href="/webmail/"
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex min-h-[2.75rem] items-center rounded-lg border border-border-strong px-3 py-1.5 text-sm font-medium text-fg hover:bg-surface-2 sm:min-h-0"
                data-open-webmail-link=""
            >
                {t('life.webmail.open')}
            </a>
            <span className="min-w-0 break-words text-xs text-fg-muted">{t('life.webmail.openHint', { url })}</span>
        </div>
    );
}
