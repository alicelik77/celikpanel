import { useMemo } from 'react';
import { useI18n } from '../i18n';
import { answerWasLost } from '../lib/requestIdentity';
import {
    PANEL_UPDATE_DISCOVERY_URL, decodePanelUpdateDiscovery, publishPanelUpdateDiscovery, refreshPanelUpdateDiscovery, type PanelUpdateDiscovery,
} from '../lib/panelUpdateDiscovery';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

// The owner's switch for the automatic update check, under the update card in
// Settings (alpha.132, owner 8 Nov 2026: nothing is saved by clicking a
// setting). Turning it on or off is marked and saved with the save bar; Save
// changes this setting only, and the answer is the stored result again. The
// update itself (starting, following, recovering) is not here and does not
// change.
//
// Otomatik güncelleme denetimi anahtarı: değişiklik işaretlenir ve yalnız
// Kaydet ile kaydedilir; güncellemenin kendisine dokunulmaz.
export function PanelUpdateAutoCheck({ stored }: { stored: PanelUpdateDiscovery | null }) {
    const { t } = useI18n();
    const say = useSaveWords();
    const known = useMemo(() => (stored ? { autoCheck: stored.autoCheck } : null), [stored]);
    const draft = useDraft(known);
    const on = draft.values?.autoCheck ?? false;
    const changed = draft.isChanged('autoCheck');

    const save = () => void draft.save(async (changes) => {
        const next = changes.autoCheck === true;
        let response: Response;
        try {
            response = await fetch(PANEL_UPDATE_DISCOVERY_URL, {
                method: 'PUT', cache: 'no-store', credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ auto_check: next }),
            });
        } catch {
            return 'unknown';
        }
        // A gateway's answer in the Panel's place is no answer.
        if (answerWasLost(response)) return 'unknown';
        if (!response.ok) return 'refused';
        let value: PanelUpdateDiscovery | null = null;
        try {
            value = decodePanelUpdateDiscovery(await response.json(), Date.now());
        } catch {
            value = null;
        }
        // Accepted, but the answer could not be read: what is stored is not known.
        if (!value) return 'unknown';
        publishPanelUpdateDiscovery(value);
        // The Panel sets the next check right after saving; one read
        // shortly after shows its time.
        if (next) window.setTimeout(() => { void refreshPanelUpdateDiscovery(); }, 1500);
        return 'saved';
    });

    return (
        <div className="mt-5 border-t border-border pt-4" data-update-auto="">
            <label className={`-mx-2 flex cursor-pointer items-start gap-3 rounded-r px-2 py-1 ${changed ? changedRowClass : ''}`}>
                <input
                    type="checkbox"
                    role="switch"
                    className="mt-1 h-4 w-4 shrink-0 accent-primary"
                    checked={on}
                    disabled={!stored || draft.saving}
                    aria-checked={on}
                    onChange={(event) => draft.set('autoCheck', event.currentTarget.checked)}
                />
                <span className="min-w-0">
                    <span className="block text-sm font-medium text-fg">
                        {t('panelUpdate.auto.label')}
                        {changed && <ChangedMark switched={on} />}
                    </span>
                    <span className="mt-0.5 block text-sm text-fg-muted">{t('panelUpdate.auto.help')}</span>
                    {/* Not "on" before the setting was read: unknown is said as unknown. */}
                    {!stored && <span className="mt-0.5 block text-sm text-fg-muted" data-update-auto-unknown="">{t('panelUpdate.auto.unknown')}</span>}
                </span>
            </label>
            {draft.outcome === 'refused' && <p className="mt-2 text-sm text-fg" role="alert" data-update-auto-error="">{t('panelUpdate.auto.failed')}</p>}
            {draft.isMoved('autoCheck') && draft.base && (
                <ServerMovedNote value={say(draft.base.autoCheck ? 'on' : 'off')} onTake={() => draft.take('autoCheck')} />
            )}
            <SaveBar
                className="-mx-5 px-5 pb-0"
                count={draft.count}
                saving={draft.saving}
                unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: false, onCheck: () => void refreshPanelUpdateDiscovery() } : undefined}
                onSave={save}
                onDiscard={draft.reset}
            />
        </div>
    );
}
