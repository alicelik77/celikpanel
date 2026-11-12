import type { ReactNode } from 'react';
import { BookOpen } from 'lucide-react';
import { useI18n } from '../../i18n';
import { openGuideTopic } from './context';

// A link from a screen into the guide: <GuideLink topic="domain.files.access" />
// opens the guide drawer at the chapter that claims the topic. It carries no
// guide data (the drawer fetches it), so any screen can use it. The build
// (web/scripts/build-guide.mjs) fails when a topic named here is claimed by
// no chapter.
// Ekrandan kılavuza bağlantı; konuyu sahiplenen bölümü çekmecede açar.
export function GuideLink({ topic, children, className }: { topic: string; children?: ReactNode; className?: string }) {
    const { t } = useI18n();
    return (
        <button
            type="button"
            onClick={() => openGuideTopic(topic)}
            data-guide-link={topic}
            className={className ?? 'inline-flex items-center gap-1 text-sm font-medium text-primary underline underline-offset-2 hover:text-primary-hover max-sm:min-h-11'}
        >
            <BookOpen className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            {children ?? t('guide.link')}
        </button>
    );
}
