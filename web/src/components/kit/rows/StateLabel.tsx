import { useI18n } from '../../../i18n';
import { StatusDot } from '../../ui';
import { stateTone, stateWord } from '../words';

// A domain's state as a dot with its word beside it (alpha.132): the dot
// never stands alone and the word is never a raw code (components/kit/words.ts).
// Alan adının durumu: nokta ve yanında sözcüğü; ham kod yok.
export function StateLabel({ status, admin, className }: { status: string | null | undefined; admin?: boolean; className?: string }) {
    const { t } = useI18n();
    return (
        <span className={`inline-flex items-center gap-1.5 whitespace-nowrap ${className ?? ''}`} data-state-label={status ?? ''}>
            <StatusDot tone={stateTone(status)} />
            {stateWord(t, status, { admin })}
        </span>
    );
}
