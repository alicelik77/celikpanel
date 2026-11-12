// The icon of each section of a domain's page (alpha.132), beside the
// catalogue's map in componentIcons.tsx: one place, so the row, the Overview's
// tiles and the drawings agree. A section closed by the plan is drawn with
// its own icon, faded, and the lock after its name.
// Alan adı sayfasındaki her bölümün ikonu; tek yerde tutulur.
import { AppWindow, Archive, Database, Folder, LayoutGrid, Mail, Network, ShieldCheck, Wrench } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import type { SectionId } from '../components/domain/sections';

export const domainSectionIcons: Record<SectionId, LucideIcon> = {
    overview: LayoutGrid,
    site: AppWindow,
    files: Folder,
    mail: Mail,
    databases: Database,
    dns: Network,
    security: ShieldCheck,
    backups: Archive,
    advanced: Wrench,
};
