/** POST/PUT a JSON body for a site tool (WordPress tools, malware scan); the refusal is returned whole for the screen. */
export async function sendSiteTool(url: string, method: 'POST' | 'PUT', body: unknown): Promise<Response> {
    return fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body ?? {}) });
}
