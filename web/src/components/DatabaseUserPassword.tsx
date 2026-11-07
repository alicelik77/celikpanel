import { useState } from 'react';
import { KeyRound } from 'lucide-react';
import { useDataToolsWords } from './dataToolsWords';
import { readApiError, refusalText } from '../lib/apiError';
import { followOperation, type OperationEnd } from '../lib/confirmAction';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { passwordAcceptable } from '../lib/passwordPolicy';
import { Button } from './ui';
import { PasswordInput } from './PasswordInput';

// A database user's new password (alpha.129), on the domain's Databases tab
// and on the Databases page: the shared password field (generator, show/hide,
// copy, strength; the database user rule), then the locked progress window
// with the server's own steps. The window's last line says what happens to
// the sites that use the old password: a WordPress or catalogue application
// CelikPanel installed gets the new one in its configuration file (the
// existing, file-checked mechanism), anything else must be given it by hand.
// The password typed or generated here is the caller's; the server never
// sends it back.
//
// Veritabanı kullanıcısının yeni parolası: ortak parola alanı, sonra sunucu
// adımlarıyla kilitli pencere; son satır eski parolayı kullanan siteleri söyler.

export interface DatabaseUserPasswordProps {
    /** The route that changes it (POST {password}). */
    url: string;
    user: string;
    onClose: () => void;
    /** Called after any answer, so the lists are read again. */
    onSettled?: () => void;
}

export function DatabaseUserPassword({ url, user, onClose, onSettled }: DatabaseUserPasswordProps) {
    const { t } = useDataToolsWords();
    const [password, setPassword] = useState('');
    const [busy, setBusy] = useState(false);
    const acceptable = passwordAcceptable(password, 'database_user');

    const run = async () => {
        if (!acceptable || busy) return;
        setBusy(true);
        const sent = password;
        await followOperation({
            progressTitle: t('dbu.title', { user }),
            action: async (report): Promise<OperationEnd> => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'dbUserPassword', ['check!', 'password', 'records'], { user }), t, `dbUserPassword.${url}`,
                        (headers) => fetch(url, { method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify({ password: sent }) }));
                    onSettled?.();
                    if (!res.ok) {
                        const problem = await readApiError(res);
                        return { state: res.status < 500 ? 'blocked' : 'failed', reason: refusalText(problem, t, 'dbu.failed') };
                    }
                    setPassword('');
                    onClose();
                    // The change was made (2xx); an answer that cannot be read
                    // only loses the note about the application.
                    let answer: Record<string, unknown> = {};
                    try {
                        answer = (await res.json()) as Record<string, unknown>;
                    } catch {
                        return { state: 'done', message: t('dbu.done', { user }) };
                    }
                    const domain = typeof answer.app_domain === 'string' ? answer.app_domain : '';
                    const app = answer.app_kind === 'wordpress' ? 'WordPress' : typeof answer.app_kind === 'string' ? answer.app_kind : '';
                    if (domain && answer.app_updated === true) return { state: 'done', message: t('dbu.doneApp', { user, app, domain }) };
                    if (domain) return { state: 'done', message: t('dbu.doneAppKept', { user, app, domain, detail: String(answer.app_detail ?? '') }) };
                    return { state: 'done', message: t('dbu.done', { user }) };
                } catch {
                    onSettled?.();
                    return { state: 'unknown', reason: t('dbu.unknown', { user }) };
                }
            },
        });
        setBusy(false);
    };

    return (
        <div className="space-y-2 rounded-lg border border-border bg-surface-2/50 p-3 text-sm" data-database-user-password-form={user}>
            <p className="font-semibold text-fg">{t('dbu.newPasswordFor', { user })}</p>
            <p className="text-xs text-fg-muted">{t('dbu.newPasswordHint')}</p>
            <PasswordInput field="database_user" value={password} onChange={setPassword} maxLength={128} autoComplete="new-password" />
            <div className="flex flex-wrap gap-2">
                <Button variant="primary" icon={KeyRound} disabled={!acceptable || busy} onClick={() => void run()}>{t('dbu.change')}</Button>
                <Button disabled={busy} onClick={onClose}>{t('common.cancel')}</Button>
            </div>
        </div>
    );
}
