import { useState } from 'react';
import { KeyRound } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { readApiError, type ApiError } from '../lib/apiError';
import { passwordAcceptable } from '../lib/passwordPolicy';
import { PasswordInput } from './PasswordInput';
import { Button, Dialog, ErrorBanner } from './ui';

// Self-service password change for the signed-in user; the current password
// must be proven (the API enforces it too).
// Oturumdaki kullanıcı için self-servis parola değişimi; mevcut parola
// kanıtlanmalıdır (API de bunu zorlar).
export function ChangePasswordModal({ onClose }: { onClose: () => void }) {
    const { t } = useI18n();
    const [current, setCurrent] = useState('');
    const [next, setNext] = useState('');
    const [next2, setNext2] = useState('');
    const [saving, setSaving] = useState(false);
    // A refusal is said once, in the dialogue that stays open.
    const [problem, setProblem] = useState<ApiError | null>(null);
    const mismatch = next2 !== '' && next !== next2;
    const ready = !saving && current !== '' && passwordAcceptable(next, 'account') && next === next2;

    const submit = async () => {
        if (!ready) return;
        setSaving(true);
        setProblem(null);
        try {
            const res = await fetch('/api/v1/auth/password', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ current_password: current, new_password: next }),
            });
            if (res.status === 403) {
                setProblem({ message: t('profile.wrongCurrent') });
                return;
            }
            if (!res.ok) {
                setProblem(await readApiError(res));
                return;
            }
            showToast('success', t('profile.changed'));
            onClose();
        } catch {
            setProblem({ message: t('common.resultUnknown') });
        } finally {
            setSaving(false);
        }
    };

    return (
        <Dialog
            id="change-password"
            icon={KeyRound}
            width="sm"
            title={t('profile.changePassword')}
            busy={saving}
            onDismiss={onClose}
            actions={
                <>
                    <Button onClick={onClose}>{t('users.cancel')}</Button>
                    <Button variant="primary" onClick={submit} disabled={!ready}>
                        {t('profile.changePassword')}
                    </Button>
                </>
            }
        >
                <div className="space-y-3">
                    <label className="block">
                        <span className="mb-1 block text-xs text-fg-muted">{t('profile.current')}</span>
                        <PasswordInput value={current} onChange={setCurrent} autoComplete="current-password" autoFocus />
                    </label>
                    <label className="block">
                        <span className="mb-1 block text-xs text-fg-muted">{t('profile.new')}</span>
                        <PasswordInput field="account" value={next} onChange={setNext} onGenerated={setNext2} maxLength={1024} />
                    </label>
                    <label className="block">
                        <span className="mb-1 block text-xs text-fg-muted">{t('profile.new2')}</span>
                        <PasswordInput
                            value={next2}
                            onChange={setNext2}
                            autoComplete="new-password"
                            maxLength={1024}
                            aria-invalid={mismatch}
                            onKeyDown={(e) => e.key === 'Enter' && void submit()}
                        />
                    </label>
                    {mismatch && <p role="alert" className="text-xs text-danger">{t('profile.mismatch')}</p>}
                    <ErrorBanner error={problem} />
                </div>
        </Dialog>
    );
}
