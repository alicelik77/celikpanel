// The webmail update check (alpha.107, D-038 §9): GET
// /api/v1/service/roundcube/update. Read by the webmail page's card and the
// Components row alike, through the same address.
// Web postası güncelleme denetimi; kart ve satır aynı adresi okur.
export const ROUNDCUBE_UPDATE_URL = '/api/v1/service/roundcube/update';

export interface RoundcubeUpdateCheck {
    installed: boolean;
    version?: string;
    line?: string;
    latest_on_line?: string;
    latest_overall?: string;
    update_available: boolean;
    checked_at?: string;
    error?: string;
}

export function decodeRoundcubeUpdate(raw: unknown): RoundcubeUpdateCheck {
    const body = raw as { state?: string; check?: RoundcubeUpdateCheck } | null;
    if (!body || body.state !== 'read' || !body.check || typeof body.check.installed !== 'boolean' || typeof body.check.update_available !== 'boolean') {
        throw new Error('shape');
    }
    return body.check;
}
