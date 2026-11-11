import { useRef, useState } from 'react';
import { Plus, Trash2, Globe } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { Button, FormSection, RemoteGate, ResultUnknown, inputClass } from './ui';
import { apiErrorText, readApiError } from '../lib/apiError';
import { decodeList, lastKnown, useRemote } from '../lib/remote';
import { useLostAnswer } from '../lib/lostAnswer';
import { confirmAction } from '../lib/confirmAction';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

interface DomainGeneralSettingsProps {
    domainId: number;
    domainName: string;
}

interface GeneralSettings {
    document_root: string;
    web_server: string;
    redirect_www: boolean;
    redirect_www_available: boolean;
    aliases: string[];
}

// The general settings of one domain. The form below is built from these and
// saves them back, so an answer that lacks one of them is not the contract: it
// must not become a form of defaults.
// Bir alan adının genel ayarları. Aşağıdaki form bunlardan kurulur ve bunları
// geri kaydeder; birini taşımayan yanıt sözleşme değildir, varsayılanlardan
// kurulmuş bir forma dönüşemez.
function decodeGeneralSettings(raw: unknown): GeneralSettings {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const body = raw as Record<string, unknown>;
    if (
        typeof body.document_root !== 'string' ||
        typeof body.web_server !== 'string' ||
        typeof body.redirect_www !== 'boolean' ||
        typeof body.redirect_www_available !== 'boolean'
    ) {
        throw new Error('field');
    }
    return {
        document_root: body.document_root,
        web_server: body.web_server,
        redirect_www: body.redirect_www,
        redirect_www_available: body.redirect_www_available,
        aliases: decodeList<string>(body.aliases),
    };
}

export function DomainGeneralSettings({ domainId, domainName }: DomainGeneralSettingsProps) {
    const { t } = useI18n();
    const say = useSaveWords();
    const general = useRemote(`/api/v1/domains/${domainId}/general`, decodeGeneralSettings);
    const answer = useLostAnswer(() => general.retry());
    // alpha.132: the switch is a draft over the settings last read. Nothing is
    // sent until Save; Discard puts the server's value back; leaving with the
    // switch changed asks first. A newer answer keeps the person's change and
    // marks it when the server changed the same setting meanwhile. A save
    // whose answer was lost keeps it too, and the settings read again decide
    // (useDraft drops the change once the server holds it).
    // Anahtar, son okunan ayarların üzerindeki bir taslaktır; Kaydet'e dek
    // hiçbir şey gönderilmez.
    const draft = useDraft(lastKnown(general.remote)?.value);
    const [changingAlias, setChangingAlias] = useState(false);
    const [newAlias, setNewAlias] = useState('');
    const aliasTyped = useRef('');
    aliasTyped.current = newAlias;
    // A change is sent only for settings the server sent and is not being
    // asked about again.
    // Değişiklik yalnız sunucunun gönderdiği ve yeniden sorulmayan ayarlar
    // için gönderilir.
    const settled = general.remote.state === 'known' && !general.reading && !answer.holding;

    const handleSave = () => void draft.save(async (_changes, values) => {
        const res = await answer.send(`/api/v1/domains/${domainId}/general`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                document_root: values.document_root,
                web_server: 'nginx',
                redirect_www: values.redirect_www,
            }),
        }, {
            // The settings read again either hold the switch as it was
            // sent, or they do not.
            // Yeniden okunan ayarlar anahtarı ya gönderildiği gibi tutar ya da tutmaz.
            shows: (read) => (read[0].value as GeneralSettings).redirect_www === values.redirect_www,
        });
        if (!res) return 'unknown';
        if (!res.ok) {
            showToast('error', apiErrorText(await readApiError(res), t, 'general.saveFailed'));
            return 'refused';
        }
        showToast('success', t('general.saved'));
        answer.settle();
        await general.retry();
        return 'saved';
    });

    const handleAddAlias = async () => {
        const alias = newAlias.trim();
        if (!alias || !settled || general.remote.state !== 'known') return;
        setChangingAlias(true);
        try {
            // If the answer is lost, the aliases that are read again decide:
            // the alias in a list that did not have it empties the field, so
            // it is not one press from being added twice; a list without it
            // leaves what was typed. An alias the list already had before
            // decides nothing.
            // Yanıt yiterse yeniden okunan takma adlar karar verir: önceden
            // olmayan takma ad listedeyse alan boşaltılır; yoksa yazılan kalır.
            const wanted = alias.toLowerCase();
            const listedBefore = general.remote.value.aliases.some((item) => item.toLowerCase() === wanted);
            const request = (confirmCertificateReissue: boolean) =>
                answer.send(`/api/v1/domains/${domainId}/aliases`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        alias,
                        confirm_certificate_reissue: confirmCertificateReissue,
                    }),
                }, {
                    shows: (read) => (listedBefore
                        ? null
                        : (read[0].value as GeneralSettings).aliases.some((item) => item.toLowerCase() === wanted)),
                    made: () => {
                        if (aliasTyped.current.trim() === alias) setNewAlias('');
                    },
                });
            let res = await request(false);
            if (!res) return;
            if (!res.ok) {
                let apiError = await readApiError(res);
                if (apiError.code === 'ALIAS_CERTIFICATE_REISSUE_REQUIRED') {
                    // The server asks before it reissues the certificate; the
                    // second request runs in the locked progress window.
                    let again = null as Response | null;
                    if (!(await confirmAction({
                        message: t('general.aliasReissueAddConfirm', { name: alias }),
                        action: async () => { again = await request(true); },
                    }))) return;
                    res = again;
                    if (!res) return;
                    if (!res.ok) apiError = await readApiError(res);
                }
                if (!res.ok) {
                    showToast('error', apiErrorText(apiError, t, 'common.error'));
                    if (apiError.code === 'ALIAS_CERTIFICATE_ACTIVATION_PENDING') {
                        await general.retry();
                    }
                    return;
                }
            }
            showToast('success', t('general.aliasAdded', { name: alias }));
            if (await issuedWaitingForOwner(res)) showToast('warning', t('ssl.issuedWaitingForOwner'));
            const notRunAddress = await aliasValidationNotRunAddress(res);
            if (notRunAddress) showToast('warning', t('ssl.validationNotRun', { address: notRunAddress }));
            answer.settle();
            setNewAlias('');
            await general.retry();
        } finally {
            setChangingAlias(false);
        }
    };

    const handleDeleteAlias = async (alias: string) => {
        if (!settled) return;
        await confirmAction({ message: t('general.confirmDeleteAlias', { name: alias }), confirmLabel: t('common.remove'), tone: 'danger',
            action: async () => {
                setChangingAlias(true);
                try {
                    const endpoint = `/api/v1/domains/${domainId}/aliases/${encodeURIComponent(alias)}`;
                    let res = await answer.send(endpoint, { method: 'DELETE' });
                    if (!res) return;
                    if (!res.ok) {
                        let apiError = await readApiError(res);
                        if (apiError.code === 'ALIAS_CERTIFICATE_REISSUE_REQUIRED') {
                            let again = null as Response | null;
                            if (!(await confirmAction({
                                message: t('general.aliasReissueDeleteConfirm', { name: alias }),
                                action: async () => { again = await answer.send(`${endpoint}?confirm_certificate_reissue=true`, { method: 'DELETE' }); },
                            }))) return;
                            res = again;
                            if (!res) return;
                            if (!res.ok) apiError = await readApiError(res);
                        }
                        if (!res.ok) {
                            showToast('error', apiErrorText(apiError, t));
                            if (apiError.code === 'ALIAS_CERTIFICATE_ACTIVATION_PENDING') {
                                await general.retry();
                            }
                            return;
                        }
                    }
                    showToast('success', t('general.aliasDeleted', { name: alias }));
                    if (await issuedWaitingForOwner(res)) showToast('warning', t('ssl.issuedWaitingForOwner'));
                    const notRunAddress = await aliasValidationNotRunAddress(res);
                    if (notRunAddress) showToast('warning', t('ssl.validationNotRun', { address: notRunAddress }));
                    answer.settle();
                    await general.retry();
                } finally {
                    setChangingAlias(false);
                }
            },
        });
    };

    return (
        <RemoteGate
            remote={general.remote}
            checking={t('general.checking')}
            failed={t('general.unknown')}
            onRetry={() => void general.retry()}
            busy={general.reading}
        >
            {(shown) => {
                const settings = shown.value;
                const canChange = settled && !shown.stale;
                const redirectWww = draft.values?.redirect_www ?? settings.redirect_www;
                const redirectChanged = draft.isChanged('redirect_www');
                return (
                    <div>
                        <ResultUnknown answer={answer} className="mb-4" />
                        <div>
                            <FormSection title={t('general.docRoot')} description={t('general.docRootHint')}>
                                <p className="break-all rounded-lg border border-border bg-surface-2 px-3 py-2 font-mono text-sm text-fg">
                                    {settings.document_root}
                                </p>
                            </FormSection>

                            <FormSection
                                title={t('general.webServer')}
                                description={t('general.webServerHintSingle')}
                            >
                                <p className="text-sm font-medium text-fg">Nginx</p>
                                {settings.web_server !== 'nginx' && (
                                    <p className="mt-1 text-xs text-warning">
                                        {t('general.webServerUnsupported', { name: settings.web_server || 'unknown' })}
                                    </p>
                                )}
                            </FormSection>

                            <FormSection title={t('general.redirects')}>
                                {settings.redirect_www_available ? (
                                    <div className={`-mx-3 rounded-r px-3 py-2 ${redirectChanged ? changedRowClass : ''}`}>
                                        <label className="flex cursor-pointer items-start gap-3">
                                            <input
                                                type="checkbox"
                                                name="redirect_www"
                                                checked={redirectWww}
                                                disabled={draft.saving}
                                                onChange={(event) => draft.set('redirect_www', event.target.checked)}
                                                className="mt-0.5 h-4 w-4 accent-primary"
                                            />
                                            <span>
                                                <span className="block text-sm text-fg">
                                                    {t('general.redirectWww')}
                                                    {redirectChanged && <ChangedMark switched={redirectWww} />}
                                                </span>
                                                <span className="block text-xs text-fg-subtle">{`${domainName} → www.${domainName}`}</span>
                                            </span>
                                        </label>
                                        {draft.isMoved('redirect_www') && draft.base && (
                                            <ServerMovedNote value={say(draft.base.redirect_www ? 'on' : 'off')} onTake={() => draft.take('redirect_www')} />
                                        )}
                                    </div>
                                ) : (
                                    <p className="text-sm text-fg-muted">{t('general.redirectWwwUnavailable')}</p>
                                )}
                                <p className="mt-3 rounded-lg border border-border bg-surface-2 px-3 py-2 text-xs text-fg-muted">
                                    {t('general.forceHttpsManaged')}
                                </p>
                            </FormSection>

                            <SaveBar
                                count={draft.count}
                                saving={draft.saving}
                                blockedBy={answer.holding ? t('common.lostStateReading') : !canChange ? say('stale') : undefined}
                                onSave={handleSave}
                                onDiscard={draft.reset}
                            />
                        </div>

                        <div className="mt-6 border-t border-border pt-5">
                            <h3 className="text-sm font-semibold text-fg">{t('general.aliases')}</h3>
                            <p className="mt-0.5 text-xs text-fg-muted">{t('general.aliasesHint')}</p>

                            <div className="mt-3 flex gap-2">
                                <input
                                    type="text"
                                    value={newAlias}
                                    onChange={(e) => setNewAlias(e.target.value)}
                                    onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), handleAddAlias())}
                                    placeholder={t('general.aliasPlaceholder')}
                                    aria-label={t('general.aliasPlaceholder')}
                                    className={inputClass}
                                />
                                <Button variant="primary" icon={Plus} onClick={handleAddAlias} disabled={!canChange || changingAlias}>
                                    {t('general.add')}
                                </Button>
                            </div>

                            {settings.aliases.length > 0 ? (
                                <div className="mt-3 space-y-2">
                                    {settings.aliases.map((alias) => (
                                        <div
                                            key={alias}
                                            className="flex items-center justify-between gap-2 rounded-lg border border-border bg-surface-2/50 px-3 py-2"
                                        >
                                            <span className="flex min-w-0 items-center gap-2 break-all font-mono text-sm text-fg">
                                                <Globe className="h-4 w-4 shrink-0 text-fg-subtle" />
                                                {alias}
                                            </span>
                                            <button
                                                onClick={() => handleDeleteAlias(alias)}
                                                disabled={!canChange || changingAlias}
                                                aria-label={`${t('common.remove')} ${alias}`}
                                                className="shrink-0 rounded-md p-1.5 text-fg-subtle transition-colors hover:bg-surface-3 hover:text-danger disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent disabled:hover:text-fg-subtle"
                                            >
                                                <Trash2 className="h-4 w-4" />
                                            </button>
                                        </div>
                                    ))}
                                </div>
                            ) : (
                                <p className="mt-3 text-center text-sm text-fg-subtle">{t('general.noAliases')}</p>
                            )}
                        </div>
                    </div>
                );
            }}
        </RemoteGate>
    );
}

// The alias change reissued the certificate, but the site's configuration file
// is the owner's and does not use it yet (D-031 step 1b). An answer that
// cannot be read says nothing more than the success it already is.
async function issuedWaitingForOwner(res: Response): Promise<boolean> {
    try {
        const body = (await res.clone().json()) as { status?: unknown } | null;
        return body?.status === 'waiting_for_owner';
    } catch {
        return false;
    }
}

// The alias change's certificate request went ahead without the probe
// (set11 cell A6): the address, or ''.
async function aliasValidationNotRunAddress(res: Response): Promise<string> {
    try {
        return validationNotRunAddress((await res.clone().json()) as IssuedAnswer | null);
    } catch {
        return '';
    }
}

// A certificate request that went ahead although the validation probe could
// not be sent from this server: the site listens on an address that is not
// this server's (set11 cell A6, 2026-10-11). The address, or ''.
type IssuedAnswer = { status?: unknown; validation_probe?: unknown; validation_unknown_reason?: unknown; validation_address?: unknown };

function validationNotRunAddress(answer: IssuedAnswer | null | undefined): string {
    if (!answer || answer.validation_probe !== 'not_run' || answer.validation_unknown_reason !== 'address_not_local') return '';
    return typeof answer.validation_address === 'string' ? answer.validation_address : '';
}
