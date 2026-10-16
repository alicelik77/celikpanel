import { useI18n } from '../i18n';

// "build" and the first twelve characters of the commit, copied exactly as
// they read (owner report 2026-10-16: the hidden full id ran into the short
// one, "build 1743ac69edb01743ac69edb0e57..."). The full commit stays for
// screen readers in a visually hidden phrase that starts with a space and that
// copying skips (user-select: none); callers also put it in the title.
// Kopyalanan metin gorundugu gibidir; tam commit ekran okuyucu ve basliktadir.
export function BuildCommit({ commit }: { commit: string }) {
    const { t } = useI18n();
    return (
        <>
            build <span aria-hidden="true">{commit.slice(0, 12)}</span>
            <span className="sr-only select-none" data-build-full="">{` (${t('common.fullCommit', { commit })})`}</span>
        </>
    );
}
