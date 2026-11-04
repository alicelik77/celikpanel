// The topic a page has on screen, and the way a screen asks for a chapter.
// Kept tiny, free of React and of the guide's data: screens import it, and
// the help menu (loaded beside the page) reads it.
// Sayfanın ekrandaki konusu ve bir ekranın bölüm istemesi; küçük tutulur.

let published: string | null = null;

/**
 * A page names what it shows now (a domain's tab: "domain.access"). Call it
 * from an effect and return what it returns, so the topic goes with the page:
 * useEffect(() => publishGuideTopic(topic), [topic]).
 */
export function publishGuideTopic(topic: string | null): () => void {
    published = topic;
    return () => {
        if (published === topic) published = null;
    };
}

export function publishedGuideTopic(): string | null {
    return published;
}

export const GUIDE_OPEN_EVENT = 'celikpanel:guide-open';

/** Opens the guide drawer at the chapter that claims this topic. */
export function openGuideTopic(topic: string): void {
    window.dispatchEvent(new CustomEvent(GUIDE_OPEN_EVENT, { detail: { topic } }));
}
