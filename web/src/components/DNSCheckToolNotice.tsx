import { useEffect, useState } from 'react';
import { useI18n } from '../i18n';

// The backup DNS server's check tool (6 Nov 2026). CelikPanel puts its
// current copy in place by itself when the server is updated; the one case
// that needs a person is a check tool someone changed by hand. Only then
// this says so, with the one command. It only reads; there is no button.
// Yedek DNS sunucusunun denetim aracı elle değiştirildiyse tek komutu gösterir.

interface CheckTool {
    engine: string;
    state: string;
    command?: string;
}

export function decodeCheckTools(raw: unknown): CheckTool[] {
    if (!raw || typeof raw !== 'object' || !Array.isArray((raw as { tools?: unknown }).tools)) return [];
    return ((raw as { tools: unknown[] }).tools).flatMap((item) => {
        if (!item || typeof item !== 'object') return [];
        const tool = item as Record<string, unknown>;
        if (typeof tool.engine !== 'string' || typeof tool.state !== 'string') return [];
        return [{ engine: tool.engine, state: tool.state, command: typeof tool.command === 'string' ? tool.command : undefined }];
    });
}

export function DNSCheckToolNotice({ className = '' }: { className?: string }) {
    const { t } = useI18n();
    const [tools, setTools] = useState<CheckTool[]>([]);
    useEffect(() => {
        let alive = true;
        fetch('/api/v1/dns/peer-inspector-tool')
            .then((response) => (response.ok ? response.json() : null))
            .then((body) => { if (alive) setTools(decodeCheckTools(body)); })
            .catch(() => { /* not an administrator, or no answer: nothing to say here */ });
        return () => { alive = false; };
    }, []);
    const changed = tools.filter((tool) => tool.state === 'owner_changed' && tool.command);
    if (changed.length === 0) return null;
    return (
        <>
            {changed.map((tool) => (
                <div key={tool.engine} role="status" data-dns-check-tool={tool.state}
                    className={`rounded-lg border border-warning bg-warning/10 p-4 text-sm text-fg ${className}`}>
                    <p>{t('dnsov.checkTool.ownerChanged')}</p>
                    <code className="mt-2 block break-all rounded bg-surface-2 p-2 font-mono text-xs">{tool.command}</code>
                </div>
            ))}
        </>
    );
}
