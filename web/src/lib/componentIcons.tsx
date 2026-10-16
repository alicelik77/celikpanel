// One map from a catalogue component to its line icon (alpha.94, critique
// item 9). The Panel's emoji "icon" field stays in the answer for older pages
// but is drawn nowhere: an emoji looks different on every system, and 🚫 and 🦠
// read as an error and a virus. A component without its own entry is drawn
// with its category's icon, so a new component never appears without one.
// Bir katalog bileşeninden çizgi ikonuna tek eşlem. Panel'in emoji alanı eski
// sayfalar için yanıtta kalır ama hiçbir yerde çizilmez. Kendi girdisi olmayan
// bileşen bölümünün ikonuyla çizilir.
import {
    Activity, ArrowLeftRight, BadgeCheck, Boxes, BrickWall, Clock, Database, Feather, FileCode2, Filter,
    FolderUp, Globe, Hexagon, Inbox, Lock, Mail, MailOpen, MemoryStick, Network, ScanSearch, Send, Shield,
    ShieldBan, Table2, Zap,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

export const categoryIcons: Record<string, LucideIcon> = {
    dns: Network,
    web: Globe,
    database: Database,
    email: Mail,
    security: Shield,
    cache: Zap,
    ftp: FolderUp,
    monitoring: Activity,
    system: Clock,
};

const componentIconMap: Record<string, LucideIcon> = {
    'php-fpm': FileCode2,
    node: Hexagon,
    nginx: ArrowLeftRight,
    apache: Feather,
    postgresql: Database,
    mariadb: Database,
    phpmyadmin: Table2,
    phppgadmin: Table2,
    postfix: Send,
    exim: Send,
    rspamd: Filter,
    spamassassin: Filter,
    dovecot: Inbox,
    roundcube: MailOpen,
    wireguard: Lock,
    certbot: BadgeCheck,
    fail2ban: ShieldBan,
    nftables: BrickWall,
    clamav: ScanSearch,
    bind: Network,
    pdns: Network,
    vsftpd: FolderUp,
    redis: MemoryStick,
    valkey: MemoryStick,
    memcached: MemoryStick,
    netdata: Activity,
    cron: Clock,
};

export function componentIcon(id: string, category: string): LucideIcon {
    return componentIconMap[id] ?? categoryIcons[category] ?? Boxes;
}

