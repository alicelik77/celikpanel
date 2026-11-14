import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import type { RemoteHandle } from '../lib/remote';
import {
    LARGE_BACKUP_BYTES, PROGRAM_AREA, areaWord, contentsBytes, holdsProgram, leftOutWords, serviceAppName,
    type BackupContents,
} from '../lib/backupContents';
import { CouldNotCheck } from './ui';
import { Guidance } from './kit/rows/Guidance';
import { sizeWord, whoWord } from './kit/words';

// What a backup of this site holds, said before one is made
// (docs/SITE-BACKUP-CONTENTS-DESIGN.md): the site's files, the program's files
// and data, a service app's data and the databases of a full backup, each
// with its size; what is left out and why; that secret settings never go into
// a backup; that a large backup takes time and room; and, when the server's
// backup disk has less room than the backup needs, who must act. A read that
// failed is said as such and never stops a backup from being made.
//
// Yedek alınmadan önce bu sitenin yedeğinin neleri içereceği, boyutlarıyla;
// dışarıda kalanlar ve nedeni; büyük yedek ve disk yeri uyarısı.
export function BackupContentsNote({ contents, databaseNames }: {
    contents: RemoteHandle<BackupContents>;
    /** The domain's databases as the server listed them; null while not known. */
    databaseNames: string[] | null;
}) {
    const { t, locale } = useI18n();
    const remote = contents.remote;
    if (remote.state === 'loading') {
        return <p className="mb-3 text-xs text-fg-muted" role="status">{t('bkc.checking')}</p>;
    }
    if (remote.state === 'unknown') {
        return <CouldNotCheck className="mb-3" text={t('bkc.unknown')} onRetry={() => void contents.retry()} busy={contents.reading} />;
    }
    const value = remote.value;
    const total = contentsBytes(value);
    const size = sizeWord(total, locale);
    const holds = [t('bkc.site', { size: sizeWord(value.siteBytes, locale) }), ...value.areas.map((area) => areaWord(t, area, locale))];
    const left = leftOutWords(t, value.leftOut, value.leftOutMore, locale);
    const apps = value.areas.filter((area) => area.name !== PROGRAM_AREA).map((area) => serviceAppName(area.name));
    return (
        <div className="mb-3 space-y-1 rounded-xl border border-border bg-surface p-4 text-sm" data-backup-contents="">
            <p className="font-medium text-fg">{t('bkc.title')}</p>
            <ul className="flex flex-wrap gap-x-4 gap-y-1 text-fg-muted">
                {holds.map((item) => <li key={item}>{item}</li>)}
            </ul>
            {databaseNames && databaseNames.length > 0 && (
                <p className="text-fg-muted">{t('bkc.databases', { names: databaseNames.join(', ') })}</p>
            )}
            {!value.complete && <p className="text-xs text-warning">{t('bkc.atLeast')}</p>}
            {left.length > 0 && <p className="text-xs text-fg-muted">{t('bkc.leftTitle')} {left.join('; ')}</p>}
            {(value.program || holdsProgram(value.areas)) && <p className="text-xs text-fg-muted">{t('bkc.secrets')}</p>}
            {apps.length > 0 && <p className="text-xs text-fg-muted">{t('bkc.appsNotRestored', { apps: apps.join(', ') })}</p>}
            {total >= LARGE_BACKUP_BYTES && <p className="text-xs text-warning" role="note">{t('bkc.large', { size })}</p>}
            {value.freeBytes !== null && total > value.freeBytes && (
                <Guidance className="mt-2" tone="blocked" live="off" title={t('bkc.space.title')}
                    why={t('bkc.space.why', { free: sizeWord(value.freeBytes, locale), size })}
                    who={whoWord(t, 'administrator')} next={t('bkc.space.next')} />
            )}
        </div>
    );
}

/** What a restore did with the site's program (the server's answer). */
export interface ProgramOutcome {
    state: string;
    reason?: string;
}

const WHY: Record<string, TranslationKey> = {
    held: 'bkc.why.held', suspended: 'bkc.why.suspended', sandbox_reduced: 'bkc.why.sandbox_reduced',
    files_unknown: 'bkc.why.files_unknown', restore_unknown: 'bkc.why.restore_unknown', gate_unread: 'bkc.why.gate_unread',
    restart_loop: 'bkc.why.restart_loop', not_listening: 'bkc.why.not_listening', start_failed: 'bkc.why.start_failed',
    oom: 'bkc.why.oom', port_taken: 'bkc.why.port_taken', listens_elsewhere: 'bkc.why.listens_elsewhere',
};

// The program after a restore that stopped it (D-024): started (said by the
// toast), did not start, left stopped and why, or not known; after a failed
// restore the same, under the refusal. A program that was not running, or a
// backup without it, needs no line.
// Geri yüklemeden sonra program: başlamadı, durdurulmuş bırakıldı ve neden,
// ya da bilinmiyor.
export function RestoreProgramOutcome({ outcome, failed = false }: { outcome: ProgramOutcome; failed?: boolean }) {
    const { t } = useI18n();
    const why = outcome.reason ? t(WHY[outcome.reason] ?? 'bkc.why.other') : undefined;
    switch (outcome.state) {
        case 'started':
            return failed ? <p className="mb-3 text-xs text-fg-muted">{t('bkc.result.failedStarted')}</p> : null;
        case 'not_started':
            return <Guidance className="mb-3" tone="failed" title={t('bkc.result.notStarted')} why={why} who={whoWord(t, 'you')} next={t('bkc.next.programs')} />;
        case 'left_stopped': {
            const administrator = outcome.reason === 'held' || outcome.reason === 'sandbox_reduced';
            return <Guidance className="mb-3" tone="blocked" title={t(failed ? 'bkc.result.failedLeftStopped' : 'bkc.result.leftStopped')} why={why}
                who={whoWord(t, administrator ? 'administrator' : 'you')} next={t('bkc.next.startLater')} />;
        }
        case 'unknown':
            return <Guidance className="mb-3" tone="unknown" title={t('bkc.result.unknown')} who={whoWord(t, 'you')} next={t('bkc.next.check')} />;
        case 'not_set_up':
            return failed ? null : <Guidance className="mb-3" tone="neutral" title={t('bkc.result.notSetUp')} who={whoWord(t, 'you')} next={t('bkc.next.setUp')} />;
        default:
            return null;
    }
}
