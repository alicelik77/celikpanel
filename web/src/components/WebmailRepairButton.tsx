import { RotateCw } from 'lucide-react';

// "Repair webmail" beside an installed Roundcube. Repair re-runs the durable
// install operation; on a healthy server it is a routine action and is drawn
// like the row's other secondary actions (Manage). It is amber only when the
// server reports a webmail problem (owner observation on alpha.92,
// 2026-10-16: a routine action drawn as a problem reads as a fault).
// Kurulu Roundcube'un yanındaki "Webmail'i onar". Sağlıklı sunucuda olağan bir
// eylemdir ve satırın diğer ikincil eylemleri (Yönet) gibi çizilir; yalnız
// sunucu bir webmail sorunu bildirdiğinde sarıdır.
const neutralClass = 'inline-flex items-center gap-1.5 rounded-lg border border-border-strong bg-surface px-2.5 py-1.5 text-xs font-medium text-fg transition-colors hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-50';
const warningClass = 'inline-flex items-center gap-1.5 rounded-lg border border-warning-mark/50 bg-warning-mark/20 px-2.5 py-1.5 text-xs font-semibold text-warning disabled:cursor-not-allowed disabled:opacity-50';

export function WebmailRepairButton({ problem, disabled, title, label, onClick }: {
    problem: boolean;
    disabled: boolean;
    title: string;
    label: string;
    onClick: () => void;
}) {
    return (
        <button
            type='button'
            onClick={onClick}
            disabled={disabled}
            title={title}
            data-tone={problem ? 'warning' : 'neutral'}
            className={problem ? warningClass : neutralClass}
        >
            <RotateCw className='h-3.5 w-3.5' />
            {label}
        </button>
    );
}
