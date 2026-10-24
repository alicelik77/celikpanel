// Confining a vendor repository, shown before it is done (v0.1.0-alpha.107
// review; D-038 §3 and §12; D-022). The dialog reads the Agent's plan: the
// exact preferences text and its SHA-256, the packages that keep following
// the repository, and how `apt-get upgrade` changes with the file in place.
// Only that text, confirmed by its SHA-256, is written; a plan that changed
// meanwhile is refused and read again.
//
// Depoyu sınırlamak: önce plan (tam metin, SHA-256, apt upgrade farkı);
// yalnız onaylanan metin yazılır.
import { useState } from 'react';
import { useI18n } from '../i18n';
import { Button, Dialog, KeepWords, RemoteGate } from './ui';
import { useRemote } from '../lib/remote';
import { LifeLine } from './LifeLine';

export interface RepoConfineChange { package: string; from?: string; before?: string; before_vendor?: boolean; after?: string; after_vendor?: boolean }
export interface RepoConfinePlan {
    state: string;
    pin_path?: string;
    text: string;
    sha256: string;
    kept?: string[];
    changes?: RepoConfineChange[];
    still_from_vendor?: string[];
    upgrades_now: number;
    upgrades_confined: number;
}

export function decodeRepoConfinePlan(raw: unknown): RepoConfinePlan {
    const body = raw as { state?: string; preview?: RepoConfinePlan; error?: string[] } | null;
    if (!body || body.state !== 'read' || !body.preview || typeof body.preview.sha256 !== 'string' || typeof body.preview.text !== 'string') {
        throw new Error(body?.state === 'failed' ? (body.error ?? []).join(' ') || 'failed' : 'shape');
    }
    return body.preview;
}

export function RepoConfineDialog({ serviceId, repoName, component, onClose, onDone }: {
    serviceId: string;
    repoName: string;
    component: string;
    onClose: () => void;
    onDone: () => void;
}) {
    const { t } = useI18n();
    const handle = useRemote(`/api/v1/repo/confine-preview?service_id=${encodeURIComponent(serviceId)}`, decodeRepoConfinePlan);
    const [busy, setBusy] = useState(false);
    const [failure, setFailure] = useState('');
    const confirm = async (plan: RepoConfinePlan) => {
        setBusy(true);
        setFailure('');
        try {
            const res = await fetch('/api/v1/repo', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ service_id: serviceId, action: 'confine', expected_sha256: plan.sha256 }),
            });
            if (res.ok) {
                onDone();
                return;
            }
            const raw = await res.text();
            let code = '';
            try {
                code = (JSON.parse(raw) as { code?: string }).code ?? '';
            } catch {
                // not JSON: the generic sentence below
            }
            setFailure(code === 'REPO_CONFINE_PLAN_CHANGED' ? t('life.repo.plan.changed') : t('life.repo.confineFailed'));
            if (code === 'REPO_CONFINE_PLAN_CHANGED') void handle.retry();
        } catch {
            setFailure(t('life.repo.plan.unknown'));
        } finally {
            setBusy(false);
        }
    };
    const plan = handle.remote.state === 'known' ? handle.remote.value : null;
    return (
        <Dialog
            id="repo-confine"
            title={t('life.repo.plan.title', { name: repoName })}
            busy={busy}
            onDismiss={onClose}
            actions={
                <>
                    <Button onClick={onClose} disabled={busy}>{t('common.cancel')}</Button>
                    <Button variant="primary" onClick={() => plan && void confirm(plan)} loading={busy} disabled={busy || !plan} data-repo-confine-confirm="">
                        {t('life.repo.plan.confirm')}
                    </Button>
                </>
            }
        >
            <RemoteGate remote={handle.remote} checking={t('life.repo.plan.checking')} failed={t('life.repo.plan.failed')} onRetry={() => void handle.retry()} busy={handle.reading}>
                {({ value }) => (
                    <div className="space-y-3 text-sm leading-6 text-fg" data-repo-plan={value.sha256.slice(0, 12)}>
                        <p><KeepWords text={t('life.repo.plan.what', { component })} /></p>
                        {(value.changes ?? []).length === 0 ? (
                            <LifeLine tone="info" attr="plan-same">{t('life.repo.plan.same', { count: value.upgrades_now })}</LifeLine>
                        ) : (
                            <div data-repo-plan-changes={(value.changes ?? []).length}>
                                <p className="font-medium">{t('life.repo.plan.changesTitle')}</p>
                                <ul className="mt-1 list-disc space-y-1 pl-5">
                                    {(value.changes ?? []).map((change) => (
                                        <li key={change.package}><KeepWords text={changeText(change, t)} /></li>
                                    ))}
                                </ul>
                            </div>
                        )}
                        {(value.still_from_vendor ?? []).length > 0 && (
                            <p className="text-fg-muted" data-repo-plan-still=""><KeepWords text={t('life.repo.plan.still', { names: (value.still_from_vendor ?? []).join(', ') })} /></p>
                        )}
                        {(value.kept ?? []).length > 0 && (
                            <p className="text-fg-muted"><KeepWords text={t('life.repo.plan.kept', { count: (value.kept ?? []).length, names: (value.kept ?? []).join(', ') })} /></p>
                        )}
                        <div>
                            <p className="text-fg-muted">{t('life.repo.plan.file', { path: value.pin_path ?? '' })}</p>
                            <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-surface-2 p-3 font-mono text-xs">{value.text}</pre>
                            <p className="mt-1 break-all font-mono text-xs text-fg-subtle">SHA-256 {value.sha256}</p>
                        </div>
                        <p className="text-fg-muted">{t('life.repo.plan.undo', { path: value.pin_path ?? '' })}</p>
                        {failure && <LifeLine tone="bad" attr="plan-failed">{failure}</LifeLine>}
                    </div>
                )}
            </RemoteGate>
        </Dialog>
    );
}

type T = ReturnType<typeof useI18n>['t'];

function changeText(change: RepoConfineChange, t: T): string {
    const vars = { package: change.package, from: change.from ?? '', before: change.before ?? '', after: change.after ?? '' };
    if (change.before && change.before_vendor && !change.after) return t('life.repo.plan.change.noLongerReplaced', vars);
    if (change.before && change.before_vendor && change.after && !change.after_vendor) return t('life.repo.plan.change.distroInstead', vars);
    if (!change.before && change.after) return t('life.repo.plan.change.newlyUpgraded', vars);
    if (change.before && !change.after) return t('life.repo.plan.change.held', vars);
    return t('life.repo.plan.change.other', vars);
}
