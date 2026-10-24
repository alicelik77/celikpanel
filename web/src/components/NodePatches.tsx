// Node.js patch notices (v0.1.0-alpha.107, D-038 §9). CelikPanel installs
// Node.js from its official releases, so the operating system never updates
// it: for every installed version this says the newer release on the same
// line, installs it beside the old one when the administrator confirms (the
// verified install), lists the sites still on the old patch and moves one
// only on request: its application restarts on the new version and must
// answer, otherwise it is put back.
//
// Node.js yama bildirimleri: aynı hattın yeni yaması, eski yamadaki siteler;
// bir site yalnız istenince, kanıtla taşınır.
import { useState } from 'react';
import { useI18n } from '../i18n';
import { Button, Dialog, KeepWords } from './ui';
import { useRemote } from '../lib/remote';
import { LifeLine, type Tone } from './LifeLine';

interface NodePatch {
    installed: string;
    line: string;
    newest?: string;
    newer: boolean;
    sites: { domain_id: number; name: string }[];
}

function decodeNodePatches(raw: unknown): NodePatch[] {
    const body = raw as { state?: string; patches?: NodePatch[] } | null;
    if (!body || body.state !== 'read' || !Array.isArray(body.patches)) throw new Error('shape');
    return body.patches;
}

type Pending = { kind: 'install'; patch: NodePatch } | { kind: 'move'; patch: NodePatch; site: NodePatch['sites'][number] };

export function NodePatches({ installed, onInstall }: { installed: string[]; onInstall: (version: string) => Promise<void> }) {
    const { t } = useI18n();
    const handle = useRemote('/api/v1/runtimes/node/patches', decodeNodePatches);
    const [pending, setPending] = useState<Pending | null>(null);
    const [busy, setBusy] = useState(false);
    const [result, setResult] = useState<{ tone: Tone; text: string } | null>(null);
    if (handle.remote.state !== 'known') return null;
    const patches = handle.remote.value.filter((patch) => patch.newer && patch.newest);
    if (patches.length === 0) return null;
    const run = async () => {
        if (!pending) return;
        setBusy(true);
        setResult(null);
        try {
            if (pending.kind === 'install') {
                await onInstall(pending.patch.newest ?? '');
            } else {
                const res = await fetch(`/api/v1/domains/${pending.site.domain_id}/app/node-version`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ version: pending.patch.newest }),
                });
                const raw = await res.text();
                let body: { error?: string; rolled_back?: boolean };
                try {
                    body = JSON.parse(raw);
                } catch (error) {
                    body = { error: raw.slice(0, 200) || String(error) };
                }
                setResult(res.ok
                    ? { tone: 'ok', text: t('life.node.moved', { site: pending.site.name, version: pending.patch.newest ?? '' }) }
                    : { tone: body.rolled_back ? 'warn' : 'bad', text: t('life.node.moveFailed', { site: pending.site.name, reason: body.error ?? '' }) });
            }
        } catch {
            setResult({ tone: 'info', text: t('life.node.unknown') });
        } finally {
            setBusy(false);
            setPending(null);
            void handle.retry();
        }
    };
    return (
        <div className="mt-3 space-y-2 border-t border-border/60 pt-3" data-node-patches={patches.length}>
            {patches.map((patch) => {
                const newestInstalled = installed.includes(patch.newest ?? '');
                return (
                    <div key={patch.installed} className="space-y-1.5" data-node-patch={patch.installed}>
                        <LifeLine tone="warn" attr={`node-newer-${patch.installed}`}>
                            {t('life.node.newer', { installed: patch.installed, newest: patch.newest ?? '' })}
                        </LifeLine>
                        {!newestInstalled && (
                            <div className="pl-6">
                                <Button onClick={() => setPending({ kind: 'install', patch })} disabled={busy} className="max-sm:min-h-11" data-node-install-newer={patch.newest}>
                                    {t('life.node.install', { newest: patch.newest ?? '' })}
                                </Button>
                            </div>
                        )}
                        {patch.sites.length > 0 && (
                            <div className="pl-6 text-sm text-fg-muted">
                                <p>{t('life.node.sitesOn', { installed: patch.installed })}</p>
                                <ul className="mt-1 space-y-1">
                                    {patch.sites.map((site) => (
                                        <li key={site.domain_id} className="flex flex-wrap items-center gap-2">
                                            <span className="font-medium text-fg">{site.name}</span>
                                            {newestInstalled && (
                                                <Button onClick={() => setPending({ kind: 'move', patch, site })} disabled={busy} className="max-sm:min-h-11" data-node-move={site.name}>
                                                    {t('life.node.move', { newest: patch.newest ?? '' })}
                                                </Button>
                                            )}
                                        </li>
                                    ))}
                                </ul>
                                {!newestInstalled && <p className="mt-1 text-fg-subtle">{t('life.node.installFirst', { newest: patch.newest ?? '' })}</p>}
                            </div>
                        )}
                    </div>
                );
            })}
            {result && <LifeLine tone={result.tone} attr="node-result"><KeepWords text={result.text} /></LifeLine>}
            {pending && (
                <Dialog
                    id="node-patch"
                    title={pending.kind === 'install'
                        ? t('life.node.installTitle', { newest: pending.patch.newest ?? '' })
                        : t('life.node.moveTitle', { site: pending.site.name, newest: pending.patch.newest ?? '' })}
                    busy={busy}
                    onDismiss={() => setPending(null)}
                    actions={
                        <>
                            <Button onClick={() => setPending(null)} disabled={busy}>{t('common.cancel')}</Button>
                            <Button variant="primary" onClick={() => void run()} loading={busy} disabled={busy} data-node-confirm="">
                                {pending.kind === 'install' ? t('life.node.installConfirm') : t('life.node.moveConfirm')}
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm leading-6 text-fg">
                        {pending.kind === 'install'
                            ? t('life.node.installBody', { installed: pending.patch.installed, newest: pending.patch.newest ?? '' })
                            : t('life.node.moveBody', { site: pending.site.name, installed: pending.patch.installed, newest: pending.patch.newest ?? '' })}
                    </p>
                </Dialog>
            )}
        </div>
    );
}
