// A Node.js site's place in the site sandbox (v0.1.0-alpha.127,
// docs/SITE-SANDBOX.md), on its Hosting tab: one line says whether the
// application runs in the sandbox or in an earlier release's protection (and
// who moves it), and a site in the sandbox can install its packages: npm ci
// (npm install without a lock file) and the project's build script, as the
// site's own account in the sandbox. The install is confirmed in the panel's
// own window; the progress window then follows it, reading only, until it
// ends. A repeated press after a lost answer reuses the request id.
//
// Node.js sitesinin site korumasındaki yeri ve korumada paket kurulumu.
import { Suspense, lazy, useState } from 'react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button } from './ui';
import { readRemote, useRemote } from '../lib/remote';
import { readApiError, apiErrorText } from '../lib/apiError';
import { confirmAction, type OperationEnd, type OperationReport } from '../lib/confirmAction';

interface UnitState {
    unit_read?: boolean;
    sandbox_generation?: number;
}

interface InstallState {
    request_id?: string;
    phase: string;
    lines?: string[];
    reason?: string;
    error?: string;
}

// alpha.128 (G19): environment variables and package scripts, fetched when shown.
const NodeSiteTools = lazy(() => import('./NodeSiteTools').then((module) => ({ default: module.NodeSiteTools })));

const SANDBOX_GENERATION = 3;
const REASONS = new Set(['install_failed', 'install_lost', 'earlier_release', 'package_json_missing', 'package_json_not_file',
    'node_missing', 'install_running', 'app_folder_not_real', 'app_folder_not_owned']);

function decodeUnit(raw: unknown): UnitState {
    if (!raw || typeof raw !== 'object' || typeof (raw as { active?: unknown }).active !== 'string') throw new Error('shape');
    return raw as UnitState;
}

function decodeInstall(raw: unknown): InstallState {
    if (!raw || typeof raw !== 'object' || typeof (raw as { phase?: unknown }).phase !== 'string') throw new Error('shape');
    return raw as InstallState;
}

const pause = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

// A request identity for one press: kept after a lost answer, so a second
// press answers the first instead of starting again (D-029).
function newRequestID(): string {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return 'node-' + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

export function NodeSiteSandbox({ domainId, domainName }: { domainId: number; domainName: string }) {
    const { t } = useI18n();
    const unit = useRemote(`/api/v1/domains/${domainId}/app/status`, decodeUnit);
    const [requestID, setRequestID] = useState(newRequestID);
    if (unit.remote.state !== 'known' || !unit.remote.value.unit_read) return null;
    const inside = (unit.remote.value.sandbox_generation ?? 0) >= SANDBOX_GENERATION;
    const url = `/api/v1/domains/${domainId}/app/packages`;

    const reasonText = (state: InstallState) => REASONS.has(state.reason ?? '')
        ? t(`nodesandbox.reason.${state.reason}` as TranslationKey)
        : t('nodesandbox.reason.unknown', { detail: state.error || state.reason || '—' });

    // Follows the step until it ends; it only reads.
    const follow = async (report: OperationReport, since: number): Promise<OperationEnd> => {
        for (;;) {
            const read = await readRemote(url, decodeInstall);
            if (read.state !== 'known') return { state: 'unknown', reason: apiErrorText(read.reason, t) };
            const state = read.value;
            const note = state.lines?.slice(-4).join(' · ');
            if (state.phase === 'running') {
                report.steps([{ id: 'npm', label: t('nodesandbox.installStep'), state: 'running', since, note }]);
                await pause(2000);
                continue;
            }
            report.steps([{ id: 'npm', label: t('nodesandbox.installStep'), state: state.phase === 'succeeded' ? 'done' : 'failed', note }]);
            if (state.phase === 'succeeded') return { state: 'done', message: t('nodesandbox.installDone') };
            return { state: state.reason === 'install_lost' ? 'unknown' : 'failed', reason: reasonText(state) };
        }
    };

    const install = async () => {
        await confirmAction({
            title: t('nodesandbox.installTitle', { site: domainName }),
            message: t('nodesandbox.installBody'),
            confirmLabel: t('nodesandbox.installConfirm'),
            progressTitle: t('nodesandbox.installProgress', { site: domainName }),
            action: async (report): Promise<OperationEnd> => {
                const since = Date.now();
                report.steps([{ id: 'npm', label: t('nodesandbox.installStep'), state: 'running', since }]);
                try {
                    const res = await fetch(url, {
                        method: 'POST', headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ request_id: requestID, build: true }),
                    });
                    if (!res.ok) {
                        setRequestID(newRequestID());
                        return { state: 'blocked', reason: apiErrorText(await readApiError(res), t) };
                    }
                    const started = (await res.json()) as InstallState;
                    if (started.reason === 'install_running') return follow(report, since);
                    if (!started.phase) {
                        setRequestID(newRequestID());
                        return { state: 'blocked', reason: reasonText(started) };
                    }
                    const end = await follow(report, since);
                    setRequestID(newRequestID());
                    return end;
                } catch {
                    // The answer was lost: the same request id is kept, so a
                    // second press answers the first start.
                    return { state: 'unknown', reason: t('nodesandbox.lost') };
                }
            },
        });
    };

    return (
        <div className="space-y-2" data-node-site-sandbox={inside ? 'inside' : 'earlier'}>
            <p className={`text-sm ${inside ? 'text-fg-muted' : 'text-warning'}`}>
                {t(inside ? 'nodesandbox.site.inside' : 'nodesandbox.site.earlier')}
            </p>
            {inside && (
                <div>
                    <Button onClick={() => void install()} className="max-sm:min-h-11" data-node-install="">
                        {t('nodesandbox.install')}
                    </Button>
                </div>
            )}
            {inside && <Suspense fallback={null}><NodeSiteTools domainId={domainId} domainName={domainName} /></Suspense>}
        </div>
    );
}
