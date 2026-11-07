import { useState } from 'react';
import { useI18n } from '../i18n';

// A command the server's administrator runs on the server, with one Copy
// button (alpha.127). Used where CelikPanel has no install path of its own for
// what is missing (the operating system's automatic updates, logrotate): the
// owner's own recovery route is a short terminal command (AGENTS.md,
// "User-operated recovery first"), so the command is shown whole, ready to
// paste, instead of inside a sentence. Copying runs nothing.
// Sunucuda root olarak çalıştırılacak komut ve tek Kopyala düğmesi; kopyalamak
// hiçbir şey çalıştırmaz.
export function CopyCommand({ command, label }: { command: string; label?: string }) {
    const { t } = useI18n();
    const [said, setSaid] = useState<'copied' | 'failed' | null>(null);
    const copy = async () => {
        try {
            await navigator.clipboard.writeText(command);
            setSaid('copied');
        } catch {
            setSaid('failed');
        }
    };
    return (
        <div className="mt-2 space-y-1" data-copy-command="">
            <p className="text-xs text-fg-muted">{label ?? t('adminNext.runAsRoot')}</p>
            <div className="flex flex-wrap items-start gap-2">
                <code className="min-w-0 flex-1 break-all rounded-md bg-surface-2 px-2 py-1.5 font-mono text-xs text-fg">{command}</code>
                <button type="button" onClick={() => void copy()} className="shrink-0 rounded-md border border-border-strong px-2.5 py-1 text-xs font-medium text-fg hover:bg-surface-2 max-sm:min-h-11">
                    {t('common.copy')}
                </button>
            </div>
            {said && <p role="status" className="text-xs text-fg-muted">{t(said === 'copied' ? 'adminNext.copied' : 'adminNext.copyFailed')}</p>}
        </div>
    );
}
