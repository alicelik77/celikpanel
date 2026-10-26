// The one confirmed webmail-address operation (alpha.110): set up an address
// or remove it. It follows the same discipline as the webmail update
// (components/RoundcubeUpdateFlow.tsx):
//
// - Confirming sends ONE POST that carries a request id. The server answers
//   202 with a durable operation; every later request is a GET read of it.
//   Polling never starts a second change.
// - A POST whose answer was lost may have arrived: it is looked up by its
//   request id and never sent again.
// - After a reload, an operation that is still queued or running is followed
//   again from the address read's last_operation.
// - 409 WEBMAIL_ADDRESS_NOT_READY carries the server's own fresh check (it
//   re-checked what the screen showed); nothing was started.
//
// Onaylanan tek web postası adresi işlemi: tek POST, sonra yalnız okuma; kaybolan
// yanıt istek kimliğiyle aranır, yeniden gönderilmez; yeniden yüklemede çalışan
// işleme yeniden bağlanılır.
import { useEffect, useRef, useState } from 'react';
import type { TranslationKey } from '../i18n/en';
import { newRequestId } from './requestIdentity';
import { readWebmailUpdateOperation } from './roundcubeUpdate';
import { decodeWebmailAddressCheck, decodeWebmailAddressOperation, type WebmailAddressCheck, type WebmailAddressOperation } from './webmailAddress';

export interface FlowNotice {
    tone: 'info' | 'bad';
    key: TranslationKey;
    vars?: Record<string, string>;
    /** A reason the screen says in its own words (inside startFailed). */
    reasonKey?: TranslationKey;
}

const POLL_MS = 1500;
const MAX_MISSES = 40;
const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

function requestId(): string | null {
    try {
        return newRequestId();
    } catch {
        return null;
    }
}

export function useWebmailAddressOperation({ lastOperation, onChanged, onNotReady }: {
    lastOperation: WebmailAddressOperation | null;
    onChanged: () => void;
    onNotReady: (check: WebmailAddressCheck) => void;
}) {
    const [operation, setOperation] = useState<WebmailAddressOperation | null>(null);
    const [following, setFollowing] = useState(false);
    const [notice, setNotice] = useState<FlowNotice | null>(null);
    const [starting, setStarting] = useState(false);
    const followed = useRef('');
    const sending = useRef(false);
    const alive = useRef(true);
    const changed = useRef(onChanged);
    changed.current = onChanged;
    const notReady = useRef(onNotReady);
    notReady.current = onNotReady;
    useEffect(() => () => { alive.current = false; }, []);

    // follow reads the operation until it ends; it never sends a change.
    const follow = async (id: string) => {
        if (followed.current === id) return;
        followed.current = id;
        setFollowing(true);
        let misses = 0;
        while (alive.current) {
            const read = await readWebmailUpdateOperation(`id=${encodeURIComponent(id)}`);
            if (!alive.current) return;
            const next = read && read !== 'error' ? decodeWebmailAddressOperation(read) : null;
            if (next) {
                misses = 0;
                setOperation(next);
                if (next.status === 'succeeded' || next.status === 'failed') break;
            } else if (++misses >= MAX_MISSES) {
                // The panel did not answer for a minute: the work goes on on
                // the server; a reload reads it again.
                setNotice({ tone: 'info', key: 'life.addr.lostSight' });
                break;
            }
            await wait(POLL_MS);
        }
        if (!alive.current) return;
        followed.current = '';
        setFollowing(false);
        changed.current();
    };

    // Re-attach to work that is still running (a reload, another tab).
    useEffect(() => {
        if (lastOperation && (lastOperation.status === 'queued' || lastOperation.status === 'running')) void follow(lastOperation.id);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [lastOperation?.id, lastOperation?.status]);

    // start sends the one POST of one confirmed action.
    const start = async (url: string, payload: Record<string, unknown>) => {
        if (sending.current || following) return;
        sending.current = true;
        setStarting(true);
        setNotice(null);
        setOperation(null);
        try {
            const id = requestId();
            if (!id) {
                setNotice({ tone: 'bad', key: 'life.addr.startFailed', reasonKey: 'life.addr.noRequestID' });
                return;
            }
            let res: Response | null = null;
            try {
                res = await fetch(url, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ...payload, request_id: id }),
                });
            } catch {
                res = null;
            }
            if (res === null) {
                // The answer was lost: the request may have arrived. Look it up
                // by its id; never send it again.
                const found = await readWebmailUpdateOperation(`request_id=${id}`);
                const op = found && found !== 'error' ? decodeWebmailAddressOperation(found) : null;
                if (op) {
                    setOperation(op);
                    void follow(op.id);
                } else {
                    setNotice({ tone: 'info', key: 'life.addr.answerLost' });
                }
                return;
            }
            const raw = await res.text();
            let body: { operation?: unknown; error?: string | { code?: string; message?: string }; code?: string; check?: unknown } = {};
            try {
                body = JSON.parse(raw);
            } catch {
                body = { error: raw.slice(0, 200) };
            }
            const op = res.status === 202 ? decodeWebmailAddressOperation(body.operation) : null;
            if (op) {
                setOperation(op);
                void follow(op.id);
                return;
            }
            const code = typeof body.error === 'object' ? body.error?.code : body.code;
            if (res.status === 409 && code === 'WEBMAIL_ADDRESS_NOT_READY') {
                const check = decodeWebmailAddressCheck({ check: body.check });
                if (check) notReady.current(check);
                setNotice({ tone: 'info', key: 'life.addr.notReadyAgain' });
                return;
            }
            if (res.status === 409 && (code === 'service_operation_busy' || code === 'server_setup_busy')) {
                setNotice({ tone: 'info', key: 'life.addr.busy' });
                return;
            }
            const reason = typeof body.error === 'object' ? body.error?.message ?? '' : body.error ?? '';
            setNotice({ tone: 'bad', key: 'life.addr.startFailed', vars: { reason } });
        } finally {
            sending.current = false;
            if (alive.current) setStarting(false);
        }
    };

    return { operation, following, starting, notice, start, clearNotice: () => setNotice(null) };
}
