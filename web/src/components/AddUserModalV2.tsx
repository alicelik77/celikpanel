import { useState } from 'react';
import { User, Key } from 'lucide-react';
import { showToast } from './Toast';
import { Button, Dialog } from './ui';
import { readApiError, refusalText } from '../lib/apiError';
import { useI18n } from '../i18n';
import { generatedPassword, type CreatedSecret } from '../lib/generatedPassword';

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

    // A password from the browser's cryptographic generator, shown in the
    // field; copying is offered, never logged.
    const generatePassword = () => {
        const pwd = generatedPassword();
        setPassword(pwd);
        void navigator.clipboard?.writeText(pwd).then(() => showToast('success', t('dbm.generatedCopied')), () => showToast('info', t('dbm.generated')));
    };

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        setLoading(true);

        try {
            const res = await fetch(`/api/v1/database-servers/${serverId}/users`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username, password }),
            });

            if (!res.ok) {
                throw new Error(refusalText(await readApiError(res), t, 'dbm.createUserFailed'));
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
                    <Button type="submit" variant="primary" disabled={loading}>
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
                    <div className="flex gap-2">
                        <input
                            type="text"
                            value={password}
                            onChange={(e) => setPassword(e.target.value)}
                            className="flex-1 bg-surface-2 border border-border rounded-lg px-4 py-2 text-fg focus:border-primary"
                            placeholder={t('dbm.password')}
                            required
                        />
                        <button
                            type="button"
                            onClick={generatePassword}
                            className="px-4 py-2 bg-surface-3 hover:bg-surface-3 text-fg rounded-lg transition-colors flex items-center gap-2"
                            title={t('dbm.generate')}
                        >
                            <Key className="w-4 h-4" />
                            {t('dbm.generateShort')}
                        </button>
                    </div>
                    <p className="text-xs text-fg-subtle mt-1">
                        {t('dbm.generateNote')}
                    </p>
                </div>

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
