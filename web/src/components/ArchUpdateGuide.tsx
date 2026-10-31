// Security → Security updates on Arch Linux (alpha.114). Arch has no
// security-only updates, so CelikPanel offers no switch and runs no upgrade:
// the owner updates by hand. This says who acts, the steps in order, and what
// a full upgrade can change under hosted sites and databases. It only reads
// and shows text; nothing here starts anything.
//
// Arch'ta yalnız güvenlik güncellemesi yoktur; CelikPanel anahtar sunmaz ve
// yükseltme çalıştırmaz. Sahibin elle güncellemesi için adımlar ve uyarılar.
import { AlertTriangle } from 'lucide-react';
import { useI18n } from '../i18n';
import { KeepWords } from './ui';

export const ARCH_NEWS = 'https://archlinux.org/news/';
export const ARCH_UPGRADE_COMMAND = 'sudo pacman -Syu';
export const ARCH_FAILED_COMMAND = 'systemctl --failed';
export const ARCH_NGINX_COMMAND = 'sudo nginx -t';
export const ARCH_STATUS_COMMAND = 'sudo systemctl status php-fpm postfix dovecot';
export const ARCH_KERNEL_RUNNING = 'uname -r';
export const ARCH_KERNEL_INSTALLED = 'pacman -Q linux';
export const ARCH_REBOOT_COMMAND = 'sudo reboot';

export function ArchUpdateGuide() {
    const { t } = useI18n();
    return (
        <div className="space-y-3 border-t border-border/60 pt-3" data-arch-guide="">
            <p className="text-sm font-medium text-fg">{t('life.arch.title')}</p>
            <p className="text-sm text-fg-muted">{t('life.arch.intro')}</p>
            <ol className="list-decimal space-y-2 pl-9 text-sm leading-6 text-fg" data-arch-steps="">
                <li data-arch-step="news"><KeepWords text={t('life.arch.step1')} /></li>
                <li data-arch-step="upgrade"><KeepWords text={t('life.arch.step2', { command: ARCH_UPGRADE_COMMAND })} /></li>
                <li data-arch-step="check">
                    <KeepWords text={t('life.arch.step3', { failed: ARCH_FAILED_COMMAND, nginx: ARCH_NGINX_COMMAND, status: ARCH_STATUS_COMMAND })} />
                </li>
                <li data-arch-step="kernel">
                    <KeepWords text={t('life.arch.step4', { running: ARCH_KERNEL_RUNNING, installed: ARCH_KERNEL_INSTALLED, reboot: ARCH_REBOOT_COMMAND })} />
                </li>
            </ol>
            <div className="rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2" data-arch-warning="">
                <p className="flex items-center gap-2 text-sm font-medium text-fg">
                    <AlertTriangle className="h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                    {t('life.arch.warnTitle')}
                </p>
                <ul className="mt-1.5 list-disc space-y-1.5 pl-9 text-sm leading-6 text-fg">
                    <li data-arch-warn="php">{t('life.arch.warnPhp')}</li>
                    <li data-arch-warn="postgresql">{t('life.arch.warnPostgres')}</li>
                </ul>
            </div>
            <p className="text-sm text-fg-muted">{t('life.arch.done')}</p>
        </div>
    );
}
