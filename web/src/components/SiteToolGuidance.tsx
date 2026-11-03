import type { ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, HelpCircle, Loader2 } from 'lucide-react';
import { useI18n } from '../i18n';

// The guidance block of the site tools (alpha.118, D-024): the current
// reason first, then who acts, the next step and how it goes on. The tone
// tells apart a verified failure, an unmet prerequisite, an unknown result,
// work in progress and success.
// Site araçlarının yönlendirme bloğu: önce mevcut neden, sonra kim yapar,
// sonraki adım ve nasıl devam eder.

export type GuidanceTone = 'progress' | 'success' | 'failure' | 'prerequisite' | 'unknown';

const toneClass: Record<GuidanceTone, string> = {
    progress: 'border-border bg-surface-2',
    success: 'border-success/30 bg-success/10',
    failure: 'border-danger/30 bg-danger/10',
    prerequisite: 'border-warning-mark/50 bg-warning-mark/20',
    unknown: 'border-warning-mark/50 bg-warning-mark/20',
};

export function SiteToolGuidance({ tone, reason, who, next, resumes, children, className }: {
    tone: GuidanceTone;
    reason: string;
    who?: 'you' | 'administrator' | 'nobody';
    next?: string;
    resumes?: string;
    children?: ReactNode;
    className?: string;
}) {
    const { t } = useI18n();
    const Icon = tone === 'progress' ? Loader2 : tone === 'success' ? CheckCircle2 : tone === 'unknown' ? HelpCircle : AlertTriangle;
    return (
        <div role={tone === 'failure' ? 'alert' : 'status'} className={`rounded-lg border p-3 text-sm text-fg ${toneClass[tone]} ${className ?? ''}`}>
            <div className="flex items-start gap-2">
                <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${tone === 'progress' ? 'animate-spin' : ''}`} aria-hidden="true" />
                <div className="min-w-0 space-y-1">
                    <p className="break-words font-medium">{reason}</p>
                    {who && <p><span className="text-fg-muted">{t('wpt.guide.who')}: </span>{t(`wpt.who.${who}`)}</p>}
                    {next && <p><span className="text-fg-muted">{t('wpt.guide.next')}: </span>{next}</p>}
                    {resumes && <p><span className="text-fg-muted">{t('wpt.guide.resumes')}: </span>{resumes}</p>}
                    {children}
                </div>
            </div>
        </div>
    );
}

/** POST/PUT a JSON body; the refusal is returned whole for the screen. */
export async function sendSiteTool(url: string, method: 'POST' | 'PUT', body: unknown): Promise<Response> {
    return fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body ?? {}) });
}
