import sys
p = sys.argv[1]
t = open(p, encoding='utf-8').read()
anchor = "async function agentPhase(events) {\n"
assert t.count(anchor) == 1
helper = r'''// set16 rerun (2026-10-15): what the surface of a refused read looks like, measured, not judged here: the computed
// border, background and text colours of each [role="alert"] in the given root (and of every element inside it whose
// text colour is the theme's failure colour, --danger), and for each "celikpanel-agent" in its text the line boxes
// of exactly that token (one top = drawn unbroken). Light theme: --danger 179 38 30, --warning-mark 245 179 1.
const SURFACE_FACTS = (rootSelector) => {
    const root = rootSelector ? document.querySelector(rootSelector) : document;
    if (!root) return { found: false, root: rootSelector };
    const danger = getComputedStyle(document.documentElement).getPropertyValue('--danger').trim();
    const dangerRgb = danger ? 'rgb(' + danger.split(/\s+/).join(', ') + ')' : null;
    const alerts = Array.from(root.querySelectorAll('[role="alert"]')).filter(a => a.getBoundingClientRect().width > 0);
    const tokenRuns = (el, token) => {
        const out = [];
        const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
        for (let n = walker.nextNode(); n; n = walker.nextNode()) {
            let i = n.textContent.indexOf(token);
            while (i >= 0) {
                const r = document.createRange(); r.setStart(n, i); r.setEnd(n, i + token.length);
                const tops = Array.from(r.getClientRects()).filter(x => x.width > 0).map(x => Math.round(x.top));
                const parent = n.parentElement;
                out.push({ tops, unbroken: new Set(tops).size === 1, nowrap_parent: parent ? getComputedStyle(parent).whiteSpace : null });
                i = n.textContent.indexOf(token, i + token.length);
            }
        }
        return out;
    };
    return { found: true, root: rootSelector, danger_rgb: dangerRgb, alerts: alerts.map(a => {
        const s = getComputedStyle(a);
        const inDanger = Array.from(a.querySelectorAll('*')).filter(x => dangerRgb && getComputedStyle(x).color === dangerRgb).map(x => x.tagName + ':' + (x.textContent || '').trim().slice(0, 60));
        const tokens = tokenRuns(a, 'celikpanel-agent');
        return { text: (a.innerText || '').replace(/\s+/g, ' ').trim().slice(0, 300), class: a.className, data_firewall_unread: a.hasAttribute('data-firewall-unread'),
            border_color: s.borderTopColor, background_color: s.backgroundColor, color: s.color,
            failure_colour_on_alert: dangerRgb ? [s.borderTopColor, s.backgroundColor, s.color].some(c => c === dangerRgb || c.startsWith(dangerRgb.replace(')', ','))) : null,
            elements_in_failure_colour: inDanger, width: Math.round(a.getBoundingClientRect().width),
            celikpanel_agent_tokens: tokens.length, celikpanel_agent_all_unbroken: tokens.length ? tokens.every(x => x.unbroken) : null, token_runs: tokens };
    }) };
};
async function surfaceFacts(page, rootSelector = null) {
    return page.evaluate(SURFACE_FACTS, rootSelector).catch(error => ({ error: String(error.message).slice(0, 160) }));
}

'''
t = t.replace(anchor, helper + anchor)
# components (TR, desktop): surface facts after the 30 s wait
old = "    out.components_tr = { load_at: iso(at), ...(await bannerFacts(comp)), agent_sentence_tr: AGENT_TR.test(compText), agent_sentence_en: AGENT_EN.test(compText),\n"
assert t.count(old) == 1
t = t.replace(old, "    out.components_tr_surface = await surfaceFacts(comp, 'main');\n" + old)
# a 390x844 Turkish Components page with the Agent stopped (token wrap at phone width)
old2 = "    await cardLog(comp);\n    // the Dashboard in Turkish and in English with the Agent stopped"
assert t.count(old2) == 1
phone = '''    await cardLog(comp);
    {   // set16 rerun: the same page at 390x844 (Turkish), the surface and the command tokens at phone width
        const pl = 'p6-components-tr-phone-agent-stopped';
        const pp = await newPage(pl, 'tr', 'phone');
        await toFront(pp);
        await pp.goto(base + '/services', { waitUntil: 'domcontentloaded', timeout: 60000 });
        await pause(20000);
        await pp.screenshot({ path: join(dir, 'components-tr-phone-agent-stopped-viewport.png'), fullPage: false });
        await pp.screenshot({ path: join(dir, 'components-tr-phone-agent-stopped-full.png'), fullPage: true });
        out.components_tr_phone_surface = await surfaceFacts(pp, 'main');
        await pp.close();
    }
    await toFront(comp);
    // the Dashboard in Turkish and in English with the Agent stopped'''
t = t.replace(old2, phone)
old3 = "        const attention = await attentionFacts(page);\n"
assert t.count(old3) == 1
t = t.replace(old3, old3 + "        const attentionSurface = await page.evaluate(() => { const s = Array.from(document.querySelectorAll('section')).find(x => /^(Needs attention|İlgi istiyor)/.test((x.innerText || '').trim())); if (s) s.setAttribute('data-set16-attention', ''); return !!s; }).then(found => found ? surfaceFacts(page, '[data-set16-attention]') : { found: false }).catch(error => ({ error: String(error.message).slice(0, 160) }));\n")
old4 = "        out['dashboard_' + tag] = { load_at: iso(t0), attention,\n"
assert t.count(old4) == 1
t = t.replace(old4, "        out['dashboard_' + tag] = { load_at: iso(t0), attention, attention_surface: attentionSurface,\n")
open(p, 'w', encoding='utf-8', newline='').write(t)
print('ok')
