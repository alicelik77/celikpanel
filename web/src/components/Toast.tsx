import { useState, useEffect } from 'react';
import { CheckCircle, XCircle, Info, AlertTriangle, X } from 'lucide-react';
import { useI18n } from '../i18n';

export type ToastType = 'success' | 'error' | 'info' | 'warning';

/**
 * Beside the sentence (owner feedback 2026-10-31): the fix, as a button that
 * runs it (the caller's navigate, for example), and the code under Technical
 * details. The caller translates both labels.
 */
export interface ToastExtra {
    /** One toast per key: a second showToast with the same key (or, without
        a key, the same kind and sentence) while the first is on screen shows
        nothing new (owner report 6 Nov 2026: two identical toasts stacked). */
    key?: string;
    action?: { label: string; run: () => void };
    details?: { label: string; text: string };
}

interface Toast extends ToastExtra {
    id: number;
    type: ToastType;
    message: string;
}

let toastId = 0;
const toastLimit = 4;
/** Failures and warnings stay until the person closes them. */
export const toastStays = (type: ToastType) => type === 'error' || type === 'warning';
const toastListeners: ((toast: Toast) => void)[] = [];
const toastKey = (toast: Toast) => toast.key ?? toast.type + ': ' + toast.message;
const onScreen = new Set<string>();

// While a confirmed operation runs in the progress window, what it would say
// in a toast is said in the window instead (components/ConfirmHost.tsx): one
// place for the result, never the same sentence twice (owner, 6 Nov 2026).
// The newest capture (a follow-up inside an operation) takes it.
// Onaylanan iş ilerleme penceresinde sürerken bildirimi pencere söyler.
export interface CapturedToast extends ToastExtra { type: ToastType; message: string }
const captures: ((toast: CapturedToast) => void)[] = [];
export function captureToasts(sink: (toast: CapturedToast) => void): () => void {
    captures.push(sink);
    return () => {
        const at = captures.lastIndexOf(sink);
        if (at >= 0) captures.splice(at, 1);
    };
}

export function showToast(type: ToastType, message: string, extra?: ToastExtra) {
    const capture = captures[captures.length - 1];
    if (capture) {
        capture({ type, message, ...extra });
        return;
    }
    const toast: Toast = {
        id: toastId++,
        type,
        message,
        ...extra,
    };
    toastListeners.forEach(listener => listener(toast));
}

export function ToastContainer() {
    const { t } = useI18n();
    const [toasts, setToasts] = useState<Toast[]>([]);

    useEffect(() => {
        // A failure or a warning stays until it is closed (owner feedback
        // 2026-10-31): the person has to act on it, and a sentence that is
        // gone after five seconds cannot be read twice or acted on. A
        // confirmation leaves by itself. At most the four newest stay.
        // Hata ve uyari kapatilana dek kalir; onay kendiliginden gider.
        const listener = (toast: Toast) => {
            const id = toastKey(toast);
            if (onScreen.has(id)) return;
            onScreen.add(id);
            setToasts(prev => [...prev, toast].slice(-toastLimit));
            if (toastStays(toast.type)) return;
            setTimeout(() => {
                setToasts(prev => prev.filter(t => t.id !== toast.id));
            }, 5000);
        };
        toastListeners.push(listener);
        return () => {
            const index = toastListeners.indexOf(listener);
            if (index > -1) toastListeners.splice(index, 1);
        };
    }, []);

    const removeToast = (id: number) => {
        setToasts(prev => prev.filter(t => t.id !== id));
    };

    // What is on screen, by key, kept in step with the list (also when the
    // newest four push an older one out).
    useEffect(() => {
        onScreen.clear();
        for (const toast of toasts) onScreen.add(toastKey(toast));
    }, [toasts]);

    const getIcon = (type: ToastType) => {
        switch (type) {
            case 'success': return <CheckCircle className="w-5 h-5" />;
            case 'error': return <XCircle className="w-5 h-5" />;
            case 'warning': return <AlertTriangle className="w-5 h-5" />;
            case 'info': return <Info className="w-5 h-5" />;
        }
    };

    const getStyles = (type: ToastType) => {
        switch (type) {
            case 'success': return 'bg-success/90 border-success text-success-fg';
            case 'error': return 'bg-danger/90 border-danger text-danger-fg';
            case 'warning': return 'bg-warning/90 border-warning text-warning-fg';
            case 'info': return 'bg-primary/90 border-primary text-primary-fg';
        }
    };

    return (
        <div className="fixed top-4 right-4 z-50 space-y-2 max-w-md">
            {toasts.map(toast => (
                <div
                    key={toast.id}
                    role={toast.type === 'error' ? 'alert' : 'status'}
                    aria-live={toast.type === 'error' ? 'assertive' : 'polite'}
                    aria-atomic={true}
                    className={`flex items-start gap-3 p-4 rounded-lg border backdrop-blur-sm shadow-lg animate-slide-in ${getStyles(toast.type)}`}
                >
                    {getIcon(toast.type)}
                    <div className="min-w-0 flex-1">
                        <p className="text-sm font-medium">{toast.message}</p>
                        {toast.action && (
                            <button
                                type="button"
                                onClick={() => { removeToast(toast.id); toast.action!.run(); }}
                                className="mt-2 rounded border border-current px-2 py-1 text-xs font-semibold text-inherit hover:bg-black/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-current max-sm:min-h-11"
                            >
                                {toast.action.label}
                            </button>
                        )}
                        {toast.details && (
                            <details className="mt-1 text-xs opacity-90">
                                <summary className="cursor-pointer">{toast.details.label}</summary>
                                <code className="break-words">{toast.details.text}</code>
                            </details>
                        )}
                    </div>
                    <button
                        type={'button'}
                        aria-label={t('common.close')}
                        onClick={() => removeToast(toast.id)}
                        className={'rounded p-1 text-inherit transition-colors hover:bg-black/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-current'}
                    >
                        <X className="w-4 h-4" />
                    </button>
                </div>
            ))}
        </div>
    );
}
