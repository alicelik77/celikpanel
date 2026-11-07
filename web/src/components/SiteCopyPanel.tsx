import { useState } from 'react';
import { Copy } from 'lucide-react';
import { Button, inputClass } from './ui';
import { useRemote } from '../lib/remote';
import { apiErrorText, readApiError } from '../lib/apiError';
import { confirmAction, type OperationEnd } from '../lib/confirmAction';
import { followRequestProgress } from '../lib/requestProgress';
import { useWebWords } from '../i18n/webSettings';
import type { TranslationKey } from '../i18n/en';

// Copy this website into another site the person manages (alpha.128,
// PLESK-GAP G8): its files and, when chosen, one MariaDB database into a new
// database of the target. A target with content is replaced only after the
// person confirms; a backup of it is made first and named at the end. The
// copy follows the server's real steps in the locked window.
//
// Bu siteyi yönettiğiniz başka bir siteye kopyalayın.

interface Target { domain_id: number; domain: string; project_type: string }
interface Database { id: number; name: string }
interface CopyView { copyable: boolean; targets: Target[]; databases: Database[] }
interface CopyAnswer { backup?: string; database?: { name: string; user: string; password: string } }

function decodeCopy(raw: unknown): CopyView {
    const body = raw as CopyView | null;
    if (!body || typeof body.copyable !== 'boolean' || !Array.isArray(body.targets) || !Array.isArray(body.databases)) throw new Error('shape');
    return body;
}

export function SiteCopyPanel({ domainId, domainName }: { domainId: number; domainName: string }) {
    const { t } = useWebWords();
    const words = t as unknown as (key: TranslationKey, values?: Record<string, string | number>) => string;
    const url = `/api/v1/domains/${domainId}/copy`;
    const view = useRemote(url, decodeCopy);
    const [target, setTarget] = useState(0);
    const [database, setDatabase] = useState(0);
    const [shown, setShown] = useState<CopyAnswer | null>(null);
    if (view.remote.state !== 'known') return null;
    const state = view.remote.value;
    const picked = state.targets.find((item) => item.domain_id === target) ?? state.targets[0];

    const run = async (overwrite: boolean): Promise<void> => {
        if (!picked) return;
        let again = false;
        await confirmAction({
            title: t(overwrite ? 'copy.overwriteTitle' : 'copy.confirmTitle', { from: domainName, to: picked.domain }),
            message: t(overwrite ? 'copy.overwriteBody' : 'copy.confirmBody', { to: picked.domain }),
            confirmLabel: t(overwrite ? 'copy.overwrite' : 'copy.start'),
            tone: overwrite ? 'danger' : 'default',
            progressTitle: t('copy.progress', { from: domainName, to: picked.domain }),
            action: async (report): Promise<OperationEnd> => {
                const labels = [
                    { id: 'check', label: t('copy.step.check'), check: true },
                    ...(overwrite ? [{ id: 'backup', label: t('copy.step.backup') }] : []),
                    ...(database ? [{ id: 'database', label: t('copy.step.database') }] : []),
                    { id: 'files', label: t('copy.step.files') },
                ];
                const progress = followRequestProgress(report, labels, words, `copy.${domainId}`);
                try {
                    const res = await fetch(url, {
                        method: 'POST', headers: { 'Content-Type': 'application/json', ...progress.headers },
                        body: JSON.stringify({ target_domain_id: picked.domain_id, database_id: database, overwrite }),
                    });
                    progress.settle(res);
                    if (!res.ok) {
                        const error = await readApiError(res);
                        if (error.code === 'SITE_COPY_TARGET_NOT_EMPTY' && !overwrite) {
                            again = true;
                            return { state: 'blocked', reason: t('copy.notEmpty', { to: picked.domain }) };
                        }
                        return { state: res.status === 409 || res.status === 403 ? 'blocked' : 'failed', reason: `${apiErrorText(error, words)}${error.message ? ` (${error.message})` : ''}` };
                    }
                    const answer = (await res.json()) as CopyAnswer;
                    setShown(answer);
                    return { state: 'done', message: answer.backup ? t('copy.doneBackup', { to: picked.domain, backup: answer.backup }) : t('copy.done', { to: picked.domain }) };
                } catch {
                    progress.settle(null);
                    return { state: 'unknown', reason: t('copy.lost', { to: picked.domain }) };
                }
            },
        });
        // A target with content: the person may now confirm replacing it.
        if (again) await run(true);
    };

    return (
        <div className="space-y-4" data-site-copy="">
            <div>
                <div className="flex items-center gap-2">
                    <Copy className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                    <h3 className="text-base font-semibold text-fg">{t('copy.title')}</h3>
                </div>
                <p className="mt-1 text-sm text-fg-muted">{t('copy.desc')}</p>
            </div>
            {!state.copyable ? (
                <p className="rounded-lg border border-border bg-surface-2 px-3 py-2 text-sm text-fg-muted">{t('copy.notCopyable')}</p>
            ) : state.targets.length === 0 ? (
                <p className="rounded-lg border border-border bg-surface-2 px-3 py-2 text-sm text-fg-muted">{t('copy.noTargets')}</p>
            ) : (
                <div className="rounded-xl border border-border bg-surface p-4 space-y-3">
                    <label className="block text-sm font-medium text-fg">
                        <span>{t('copy.target')}</span>
                        <select value={picked?.domain_id ?? 0} onChange={(e) => setTarget(Number(e.target.value))} className={`${inputClass} mt-1`}>
                            {state.targets.map((item) => <option key={item.domain_id} value={item.domain_id}>{item.domain}</option>)}
                        </select>
                    </label>
                    {state.databases.length > 0 && (
                        <label className="block text-sm font-medium text-fg">
                            <span>{t('copy.database')}</span>
                            <select value={database} onChange={(e) => setDatabase(Number(e.target.value))} className={`${inputClass} mt-1`}>
                                <option value={0}>{t('copy.filesOnly')}</option>
                                {state.databases.map((item) => <option key={item.id} value={item.id}>{t('copy.withDatabase', { name: item.name })}</option>)}
                            </select>
                            <span className="mt-1 block text-xs font-normal text-fg-muted">{t('copy.databaseHint')}</span>
                        </label>
                    )}
                    <div className="flex justify-end"><Button type="button" variant="primary" onClick={() => void run(false)}>{t('copy.start')}</Button></div>
                </div>
            )}
            {shown?.database && (
                <div className="rounded-xl border border-warning/40 bg-warning/10 p-4 text-sm text-fg" role="status">
                    <p className="font-semibold">{t('copy.dbTitle')}</p>
                    <p className="mt-1">{t('copy.dbNote')}</p>
                    <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 font-mono text-xs">
                        <dt>{t('copy.dbName')}</dt><dd>{shown.database.name}</dd>
                        <dt>{t('copy.dbUser')}</dt><dd>{shown.database.user}</dd>
                        <dt>{t('copy.dbPassword')}</dt><dd className="break-all">{shown.database.password}</dd>
                    </dl>
                </div>
            )}
        </div>
    );
}
