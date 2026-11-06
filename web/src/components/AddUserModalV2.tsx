import { useState } from 'react';
import { User } from 'lucide-react';
import { showToast } from './Toast';
import { Button, Dialog, ErrorBanner } from './ui';
import { readApiError, type ApiError } from '../lib/apiError';
import { useI18n } from '../i18n';
import type { CreatedSecret } from '../lib/generatedPassword';
import { passwordAcceptable } from '../lib/passwordPolicy';
import { PasswordInput } from './PasswordInput';

interface AddUserModalV2Props {
    serverId: number;
    serverName: string;
    onClose: () => void;
    onSuccess: () => void;
    /** A password the server made up, answered once: the page shows it until closed. */
    onSecret: (secret: CreatedSecret) => void;
}

export function AddUserModalV2({ serverId, serverName, onClose, onSuccess, onSecret }: AddUserModalV2Props) {
    const { t } = useI18n();
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [loading, setLoading] = useState(false);
    // A refusal is said once, in the dialogue that stays open.
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const passwordOK = passwordAcceptable(password, 'database_user');

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!passwordOK) return;
        setLoading(true);
        setRefusal(null);

        try {
            const res = await fetch(`/api/v1/database-servers/${serverId}/users`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username, password }),
            });

            if (!res.ok) {
                setRefusal(await readApiError(res));
                return;
            }

            const data = await res.json();
            showToast('success', t('dbm.userCreated', { name: String(data.username ?? username) }));
            // The server answers a password only when it made one up; the one
            // typed here is not sent back. It stays on the page until closed.
            // Sunucu parolayı yalnız kendisi ürettiğinde yanıtlar; sayfada kalır.
            if (typeof data.password === 'string') onSecret({ user: String(data.username ?? username), password: data.password });

            onSuccess();
            onClose();
        } catch (err) {
            showToast('error', err instanceof Error && err.message ? err.message : t('dbm.createUserFailed'));
        } finally {
            setLoading(false);
        }
    };

    return (
        <Dialog
            id="add-database-user"
            icon={User}
            title={t('dbm.addUser')}
            description={t('dbm.server', { name: serverName })}
            busy={loading}
            onDismiss={onClose}
            onSubmit={handleSubmit}
            actions={
                <>
                    <Button type="button" variant="secondary" onClick={onClose}>
                        {t('common.cancel')}
                    </Button>
                    <Button type="submit" variant="primary" disabled={loading || !passwordOK}>
                        {loading ? t('dbm.creating') : t('dbm.createUser')}
                    </Button>
                </>
            }
        >
            <div className="space-y-4">
                {/* Username */}
                <div>
                    <label className="block text-sm font-medium text-fg-muted mb-2">
                        {t('dbm.username')}
                    </label>
                    <input
                        type="text"
                        value={username}
                        onChange={(e) => setUsername(e.target.value)}
                        className="w-full bg-surface-2 border border-border rounded-lg px-4 py-2 text-fg focus:border-primary"
                        placeholder="myapp_user"
                        required
                        pattern="[a-zA-Z0-9_]+"
                        title={t('dbm.namePattern')}
                    />
                    <p className="text-xs text-fg-muted mt-1">
                        {t('dbm.prefixNote')}
                    </p>
                </div>

                {/* Password */}
                <div>
                    <label className="block text-sm font-medium text-fg-muted mb-2">
                        {t('dbm.password')}
                    </label>
                    <PasswordInput
                        field="database_user"
                        value={password}
                        onChange={setPassword}
                        placeholder={t('dbm.password')}
                        maxLength={128}
                        required
                    />
                </div>

                <ErrorBanner error={refusal} sentence="dbm.createUserFailed" />

                {/* Info */}
                <div className="p-3 bg-primary/10 border border-primary/20 rounded-lg">
                    <p className="text-sm text-primary">
                        {t('dbm.grantNote')}
                    </p>
                </div>
            </div>
        </Dialog>
    );
}
