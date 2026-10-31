// The panel port view (alpha.114) as the server sends it, and the address of
// this panel on another port. Kept apart from the card so tests read it.
// Panel portu görünümü ve panelin başka porttaki adresi.

export const PANEL_PORT_URL = '/api/v1/panel/port';

export type PanelPortChange = {
    id: string;
    from_port: number;
    to_port: number;
    status: 'trial' | 'confirming' | 'confirmed' | 'rolled_back';
    reason_code?: string;
    reason?: string;
    firewall_note?: string;
    seconds_left?: number;
};

export type PanelPortView = {
    port: number;
    default_port: number;
    legacy_port: number;
    request_port: number;
    change: PanelPortChange | null;
    blocked: { code: string; reason: string } | null;
};

const STATUSES = ['trial', 'confirming', 'confirmed', 'rolled_back'];

export function decodePanelPortView(raw: unknown): PanelPortView {
    const value = raw as PanelPortView;
    const port = (n: unknown) => typeof n === 'number' && Number.isInteger(n) && n >= 0 && n <= 65535;
    if (!value || typeof value !== 'object' || !port(value.port) || value.port === 0 || !port(value.default_port) || !port(value.legacy_port) || !port(value.request_port)) {
        throw new Error('panel port view');
    }
    const change = value.change;
    if (change !== null && (typeof change !== 'object' || typeof change.id !== 'string' || !port(change.from_port) || !port(change.to_port) || !STATUSES.includes(change.status))) {
        throw new Error('panel port change');
    }
    const blocked = value.blocked;
    if (blocked !== null && (typeof blocked !== 'object' || typeof blocked.code !== 'string')) {
        throw new Error('panel port block');
    }
    return value;
}

/** The address of this panel on another port, as the browser reaches it now. */
export function panelAddressOn(port: number, location: Pick<Location, 'protocol' | 'hostname'> = window.location): string {
    const host = location.hostname.includes(':') ? `[${location.hostname}]` : location.hostname;
    return `${location.protocol}//${host}:${port}/settings?section=panel`;
}
