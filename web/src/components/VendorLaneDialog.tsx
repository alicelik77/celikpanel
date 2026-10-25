// The vendor lane, shown before it is written (v0.1.0-alpha.108, D-038
// amendment "two lanes"; owner, 2026-10-25: security and stability always
// come before freedom). One dialog for a fresh repository and for one that is
// already enabled: the plan's three steps — the repository and its pinned key,
// CelikPanel's confinement rule, the origin among the operating system's
// automatic updates — each with its file, its exact text and its SHA-256;
// what is already in place, what the owner's own configuration covers, and
// what blocks a step, with the administrator's next action. Only the plan
// confirmed by its SHA-256 is written; a lost answer reads the plan again and
// never sends it a second time.
//
// Satıcı yolu, yazılmadan önce: üç adım, dosyaları, tam metinleri ve SHA-256;
// yalnız onaylanan plan yazılır; kaybolan yanıtta plan yeniden okunur.
import { useState } from 'react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Button, Dialog, KeepWords, RemoteGate } from './ui';
import { LifeLine, type Tone } from './LifeLine';
import { showToast } from './Toast';
import { useRemote } from '../lib/remote';
import { decodeLaneRead, laneURL, LANE_CODES, LANE_REASONS, type LanePlan, type LaneStep } from '../lib/vendorLane';

const STEP_TONE: Record<string, Tone> = { done: 'ok', owner: 'info', todo: 'warn', blocked: 'bad' };

export function laneStepText(t: ReturnType<typeof useI18n>['t'], step: LaneStep, plan: LanePlan, component: string): string {
    const vars = { name: plan.name ?? plan.repo_id, origin: plan.origin ?? '', path: step.path ?? '', detail: step.detail ?? '', component };
    if (step.state === 'blocked') {
        return t(`lane.blocked.${LANE_REASONS.has(step.reason ?? '') ? step.reason : 'unknown'}` as TranslationKey, vars);
    }
    if (step.state === 'owner') {
        return t(`lane.owner.${step.reason === 'edited' || step.reason === 'owner_rule' ? step.reason : 'owner_config'}` as TranslationKey, vars);
    }
    const state = step.state === 'done' ? 'done' : 'todo';
    const name = ['repository', 'confinement', 'updates'].includes(step.step) ? step.step : 'repository';
    return t(`lane.step.${name}.${state}` as TranslationKey, vars);
}

export function VendorLaneDialog({ serviceId, component, onClose, onDone }: {
    serviceId: string;
    component: string;
    onClose: () => void;
    onDone: () => void;
}) {
    const { t } = useI18n();
    const handle = useRemote(laneURL(serviceId), decodeLaneRead);
    const [busy, setBusy] = useState(false);
    const [failure, setFailure] = useState('');
    const read = handle.remote.state === 'known' ? handle.remote.value : null;
    const plan = read?.state === 'read' ? read.plan : null;
    const confirm = async (current: LanePlan) => {
        if (!current.sha256) return;
        setBusy(true);
        setFailure('');
        try {
            const res = await fetch('/api/v1/repo/lane', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ service_id: serviceId, expected_sha256: current.sha256 }),
            });
            const raw = await res.text();
            let body: { code?: string; error?: string[]; completed?: string[]; effective?: boolean } = {};
            try {
                body = JSON.parse(raw);
            } catch {
                // not JSON: said as unknown below
            }
            if (res.ok) {
                showToast(body.effective === false ? 'warning' : 'success', t(body.effective === false ? 'lane.done.notEffective' : current.enabled ? 'lane.done.completed' : 'lane.done.enabled', { name: current.name ?? '' }));
                onDone();
                return;
            }
            const code = LANE_CODES.has(body.code ?? '') ? body.code : res.status === 502 ? 'LANE_RESULT_UNKNOWN' : 'LANE_ENABLE_FAILED';
            setFailure(t(`lane.code.${code}` as TranslationKey, {
                reason: (body.error ?? []).join(' '),
                completed: (body.completed ?? []).map((step) => t(`lane.stepName.${step}` as TranslationKey)).join(', '),
            }));
            // What the server holds now is read again; nothing is resent.
            void handle.retry();
        } catch {
            setFailure(t('lane.code.LANE_RESULT_UNKNOWN', { reason: '', completed: '' }));
            void handle.retry();
        } finally {
            setBusy(false);
        }
    };
    const title = plan?.enabled ? t('lane.title.complete', { name: plan.name ?? '' }) : t('lane.title.enable', { name: plan?.name ?? '', component });
    return (
        <Dialog
            id="vendor-lane"
            title={title}
            busy={busy}
            width="lg"
            onDismiss={onClose}
            actions={
                <>
                    <Button onClick={onClose} disabled={busy}>{t('common.cancel')}</Button>
                    <Button
                        variant="primary"
                        onClick={() => plan && void confirm(plan)}
                        loading={busy}
                        disabled={busy || !plan?.ready || !plan.sha256}
                        data-vendor-lane-confirm=""
                    >
                        {plan?.enabled ? t('lane.confirm.complete') : t('lane.confirm.enable')}
                    </Button>
                </>
            }
        >
            <RemoteGate remote={handle.remote} checking={t('lane.checking')} failed={t('lane.unknown')} onRetry={() => void handle.retry()} busy={handle.reading}>
                {({ value }) => value.state !== 'read' ? <p className="text-sm text-fg-muted">{t('lane.none')}</p> : (
                    <div className="space-y-3 text-sm leading-6 text-fg" data-vendor-lane={value.plan.repo_id} data-lane-ready={value.plan.ready ? 'yes' : 'no'}>
                        <p><KeepWords text={t(value.plan.enabled ? 'lane.intro.enabled' : 'lane.intro.fresh', { name: value.plan.name ?? '', component })} /></p>
                        <ol className="space-y-3">
                            {value.plan.steps.map((step) => (
                                <li key={step.step} data-lane-step={step.step} data-lane-state={step.state}>
                                    <LifeLine tone={STEP_TONE[step.state] ?? 'info'} attr={`lane-${step.step}`}>
                                        <KeepWords text={laneStepText(t, step, value.plan, component)} />
                                    </LifeLine>
                                    {step.state === 'todo' && step.text && (
                                        <div className="mt-1 pl-6">
                                            {step.path && <p className="break-all font-mono text-xs text-fg-subtle">{step.path}</p>}
                                            <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-surface-2 p-3 font-mono text-xs">{step.text}</pre>
                                            {step.sha256 && <p className="mt-1 break-all font-mono text-xs text-fg-subtle">SHA-256 {step.sha256}</p>}
                                        </div>
                                    )}
                                </li>
                            ))}
                        </ol>
                        {(value.plan.kept ?? []).length > 0 && (
                            <p className="text-fg-muted"><KeepWords text={t('lane.kept', { names: (value.plan.kept ?? []).join(', ') })} /></p>
                        )}
                        <p className="text-fg-muted"><KeepWords text={t('lane.after')} /></p>
                        {serviceId === 'php-fpm' && <p className="text-fg-muted"><KeepWords text={t('lane.afterTools')} /></p>}
                        {!value.plan.ready && <LifeLine tone="info" attr="lane-not-ready"><KeepWords text={t(value.plan.steps.some((s) => s.state === 'blocked') ? 'lane.notReady.blocked' : 'lane.notReady.nothing')} /></LifeLine>}
                        {value.plan.sha256 && <p className="break-all font-mono text-xs text-fg-subtle" data-lane-sha="">{t('lane.planSha', { sha: value.plan.sha256 })}</p>}
                        {failure && <LifeLine tone="bad" attr="lane-failed"><KeepWords text={failure} /></LifeLine>}
                    </div>
                )}
            </RemoteGate>
        </Dialog>
    );
}
