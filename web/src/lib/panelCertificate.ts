// What the Panel says about the certificate it serves (GET /api/v1/panel/certificate),
// and the panel name that certificate is for. One decoder for every screen that
// reads it: the Settings card and the panel port card share one remote entry.
// `https_enabled: false` is the server's answer when it found no certificate it
// could read; that is not "a trusted certificate".
//
// Panelin sunduğu sertifika ve onun geçerli olduğu panel adı. Bu adresi okuyan
// her ekran için tek çözücü; Ayarlar kartı ile panel bağlantı noktası kartı
// aynı okumayı paylaşır.
export interface PanelCertificateInfo {
    https_enabled: boolean;
    self_signed: boolean;
    issuer: string;
    expires_at: string;
    subject: string;
    dns_names: string[];
}

export const PANEL_CERTIFICATE_URL = '/api/v1/panel/certificate';

export function decodePanelCertificate(raw: unknown): PanelCertificateInfo {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const body = raw as Record<string, unknown>;
    if (typeof body.https_enabled !== 'boolean' || typeof body.self_signed !== 'boolean') throw new Error('field');
    return {
        https_enabled: body.https_enabled,
        self_signed: body.self_signed,
        issuer: typeof body.issuer === 'string' ? body.issuer : '',
        expires_at: typeof body.expires_at === 'string' ? body.expires_at : '',
        subject: typeof body.subject === 'string' ? body.subject : '',
        dns_names: Array.isArray(body.dns_names) ? body.dns_names.filter((name): name is string => typeof name === 'string') : [],
    };
}

/**
 * The panel name of a trusted certificate: a full host name, not an address and
 * not a wildcard. Empty when the panel has no trusted certificate (the browser
 * is then reached by its address only).
 */
export function panelCertificateName(info: Pick<PanelCertificateInfo, 'https_enabled' | 'self_signed' | 'subject' | 'dns_names'> | null | undefined): string {
    if (!info || !info.https_enabled || info.self_signed) return '';
    const usable = (name: string) => name.includes('.') && !name.startsWith('*') && !name.includes(':') && !/^[\d.]+$/.test(name) && !/\s/.test(name);
    const names = [...info.dns_names, info.subject].map((name) => name.trim().toLowerCase()).filter(usable);
    return names[0] ?? '';
}
