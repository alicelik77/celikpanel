import { useI18n } from '../i18n';
import { Guidance } from './kit/rows/Guidance';
import { whoWord } from './kit/words';
import { encodingLabel, type DatabaseEncodingWarning as Warning } from '../lib/domainDatabases';

// The one warning of a PostgreSQL database that is not UTF-8 (20 Nov 2026),
// on its own card, once: what it means, who acts and the way out through the
// card's own buttons (Delete and create again when empty; Copy, which makes a
// UTF-8 copy and checks every letter, when it holds data). Someone who cannot
// use those buttons reads who can. It is part of the card's reading order,
// so it is not announced on every read of the list.
//
// UTF-8 olmayan bir PostgreSQL veritabanının tek uyarısı, kendi kartında:
// anlamı, kimin yapacağı ve kartın kendi düğmeleriyle çıkış yolu.
export function DatabaseEncodingWarning({ kind, encoding, canAct }: { kind: Warning; encoding: string; canAct: boolean }) {
    const { t } = useI18n();
    const name = encodingLabel(encoding);
    return (
        <Guidance
            tone="blocked"
            live="off"
            className="mt-3"
            data-database-encoding-warning={kind}
            title={t(kind === 'unchecked' ? 'dbenc.title.unchecked' : 'dbenc.title.limited', { encoding: name })}
            why={t(kind === 'unchecked' ? 'dbenc.why.unchecked' : 'dbenc.why.limited', { encoding: name })}
            who={canAct ? whoWord(t, 'you') : t('dbenc.who.manager')}
            next={t('dbenc.next')}
            then={t('dbenc.then')}
        />
    );
}
