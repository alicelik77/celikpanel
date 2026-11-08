// One map from a catalogue component to its line icon (alpha.94, critique
// item 9). The Panel's emoji "icon" field stays in the answer for older pages
// but is drawn nowhere: an emoji looks different on every system, and 🚫 and 🦠
// read as an error and a virus. A component without its own entry is drawn
// with its category's icon, so a new component never appears without one.
// Bir katalog bileşeninden çizgi ikonuna tek eşlem. Panel'in emoji alanı eski
// sayfalar için yanıtta kalır ama hiçbir yerde çizilmez. Kendi girdisi olmayan
// bileşen bölümünün ikonuyla çizilir.
import {
    Activity, ArrowLeftRight, BadgeCheck, Boxes, Braces, BrickWall, Clock, Database, DatabaseZap, FileCode2,
    Filter, FolderCog, FolderUp, Gauge, GitBranch, Globe, Inbox, Lock, Mail, MailOpen, Mails, MailWarning,
    MemoryStick, Network, ScanSearch, Send, Shield, ShieldBan, Signpost, Table2, Wrench, Zap,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { EngineMariaDB, EnginePostgreSQL } from './engineIcons';
import { MarkGo, MarkKeyValue, MarkNameServer, MarkPython, MarkRust, MarkWebTable } from './componentMarks';

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
    devtools: Wrench,
};

// Every catalogue component (internal/core/managed_services.go) has its own
// entry, and no two share one (alpha.130, owner: "the components do not have
// proper icons"; tests/component-icons-alpha130.test.mjs keeps it so). A
// lucide icon where one says what the component does; our own drawing
// (componentMarks.ts, engineIcons.ts) where none does. Never a project's
// logo: Apache's feather and Node.js's hexagon left in alpha.130.
// Her katalog bileşeninin kendi ikonu var; ikisi aynı ikonu paylaşmaz. Hiçbiri
// bir projenin logosu değildir.
const componentIconMap: Record<string, LucideIcon> = {
    // Web: the server, the proxy in front of it, and what runs the sites.
    nginx: ArrowLeftRight,
    apache: FolderCog,
    'php-fpm': FileCode2,
    node: Braces,
    python: MarkPython,
    git: GitBranch,
    // Databases and their web tools.
    postgresql: EnginePostgreSQL,
    mariadb: EngineMariaDB,
    phpmyadmin: Table2,
    phppgadmin: MarkWebTable,
    // Mail: sending, limits, filters, mailboxes, webmail.
    postfix: Send,
    exim: Mails,
    postfwd: Gauge,
    rspamd: Filter,
    spamassassin: MailWarning,
    dovecot: Inbox,
    roundcube: MailOpen,
    // Security.
    wireguard: Lock,
    certbot: BadgeCheck,
    fail2ban: ShieldBan,
    nftables: BrickWall,
    clamav: ScanSearch,
    // DNS: two name servers, each its own.
    bind: Signpost,
    pdns: MarkNameServer,
    // FTP, caches, monitoring, system.
    vsftpd: FolderUp,
    redis: MarkKeyValue,
    valkey: DatabaseZap,
    memcached: MemoryStick,
    netdata: Activity,
    cron: Clock,
    // The administrator's own toolchains.
    go: MarkGo,
    rust: MarkRust,
};

export function componentIcon(id: string, category: string): LucideIcon {
    return componentIconMap[id] ?? categoryIcons[category] ?? Boxes;
}

