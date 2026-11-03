import { decodeListIn } from './remote';

// The subscriptions the signed-in account may see: GET /api/v1/subscriptions.
// One address, one decoder, for every screen that reads it. An answer without
// the list is not "no subscriptions"; it is not the contract.
//
// Oturum açan hesabın görebildiği abonelikler. Bu adresi okuyan her ekran için
// tek adres, tek çözücü. Listeyi taşımayan yanıt "abonelik yok" değildir.
export interface SubscriptionUsage {
    disk_used_bytes: number;
    disk_limit_bytes: number;
    domains: number;
    domains_limit: number;
    databases: number;
    databases_limit: number;
    mail_accounts: number;
    mail_limit: number;
    /** alpha.118 (D-044): this month's website traffic; absent from older servers. */
    traffic_used_bytes?: number;
    traffic_limit_bytes?: number;
    traffic_period?: string;
    traffic_measure?: { state: 'measuring' | 'unknown' | 'none'; reason?: string; last_collected?: string; collect_error?: string };
    traffic_limited?: boolean;
    /** The disk measurement in parts and the 7-day countdown. */
    disk?: {
        web_bytes: number;
        mail_bytes: number;
        db_bytes: number;
        state: 'measured' | 'partial' | 'unknown' | 'not_measured_yet';
        detail?: string;
        measured_at?: string;
        over_since?: string;
        suspend_on?: string;
        uploads_blocked: boolean;
        /** FTP/SFTP and PHP were brought to uploads_blocked (alpha.118). */
        uploads_applied?: boolean;
        /** ftp_kept, php_kept, ssh_open (comma-separated) or "error:<detail>". */
        uploads_note?: string;
    };
}

export interface SubscriptionRow {
    id: number;
    name: string;
    owner: string;
    usage?: SubscriptionUsage;
}

export const SUBSCRIPTIONS_URL = '/api/v1/subscriptions';

export function decodeSubscriptions(raw: unknown): SubscriptionRow[] {
    return decodeListIn<SubscriptionRow>(raw, 'subscriptions');
}
