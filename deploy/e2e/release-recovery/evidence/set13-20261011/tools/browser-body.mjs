async function takePool(page, events, label) {
    const first = await page.$('[data-site-config-file="php_pool"] [data-site-config-take]');
    if (!first) { await jsonl(events, { at: iso(), ev: 'take-button', label, found: false }); return false; }
    await first.click();
    await pause(800);
    const confirm = await page.$('[data-site-config-file="php_pool"] [data-site-config-take]');
    const before = await page.evaluate(() => ({ scrollY: Math.round(window.scrollY), innerHeight: window.innerHeight }));
    await confirm.click();
    await jsonl(events, { at: iso(), ev: 'take-confirmed', label, before });
    await pause(12000);
    return true;
}
async function noticeFacts(page) {
    return page.evaluate(() => {
        const box = el => { if (!el) return null; const r = el.getBoundingClientRect(); return { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right), height: Math.round(r.height) }; };
        const refusal = document.querySelector('[data-site-config-refusal]');
        const notice = document.querySelector('[data-site-config-notice]');
        const vh = window.innerHeight, vw = window.innerWidth;
        const b = box(refusal);
        return {
            viewport: { width: vw, height: vh, scrollY: Math.round(window.scrollY) },
            refusal: refusal ? { role: refusal.getAttribute('role'), restore: refusal.getAttribute('data-site-config-refusal'), text: refusal.innerText.trim().slice(0, 700), box: b,
                top_in_view: b ? b.top >= 0 && b.top < vh : false, fully_in_view: b ? b.top >= 0 && b.bottom <= vh && b.left >= 0 && b.right <= vw : false } : null,
            notice_box: box(notice),
            take_button_box: box(document.querySelector('[data-site-config-file="php_pool"] [data-site-config-take]')),
            password_fields_with_text: Array.from(document.querySelectorAll('input[type="password"]')).filter(i => i.value).length,
        };
    }).catch(error => ({ error: String(error.message).slice(0, 200) }));
}
async function badgeFacts(page, domain) {
    return page.evaluate(name => {
        const badges = Array.from(document.querySelectorAll('[data-site-config-badge]')).map(n => {
            const row = n.closest('tr, li, [role="row"]') || n.parentElement;
            const r = n.getBoundingClientRect();
            return { state: n.getAttribute('data-site-config-badge'), file: n.getAttribute('data-site-config-badge-file'), text: n.innerText.trim(),
                row_text: (row?.innerText || '').trim().slice(0, 200), visible: r.width > 0 && r.height > 0 };
        });
        return { badges, for_domain: badges.filter(b => b.row_text.includes(name)),
            password_fields_with_text: Array.from(document.querySelectorAll('input[type="password"]')).filter(i => i.value).length };
    }, domain).catch(error => ({ error: String(error.message).slice(0, 200) }));
}
async function viewShot(page, dir, name, events, facts) {
    await page.screenshot({ path: join(dir, `${name}-viewport.png`), fullPage: false });
    await page.screenshot({ path: join(dir, `${name}-full.png`), fullPage: true });
    await writeFile(join(dir, `${name}.txt`), redact(await pageText(page)));
    await jsonl(events, { at: iso(), ev: 'shot', name, facts });
}

async function cellUI(dir) {
    const events = join(dir, 'events.jsonl');
    const domain = (hand.domains || [])[0]?.name;
    const signer = await newPage(dir, 'sign-in');
    await signIn(signer, dir);
    await signer.close();
    // 1. the refused take: owner-edited pool + an owner's pool file php-fpm -t refuses
    await prep('pool-edited', events);
    await prep('broken-add', events);
    for (const [lang, vpName, viewport] of [['en', 'desktop', DESKTOP], ['tr', 'phone', PHONE]]) {
        const page = await newPage(dir, `refused-${lang}-${vpName}`, { lang, viewport });
        await domainConfig(page, domain, lang);
        await shot(page, dir, `refused-${lang}-${vpName}-1-before`, events, { lang, viewport: vpName });
        const clicked = await takePool(page, events, `${lang}-${vpName}`);
        const facts = await noticeFacts(page);
        await viewShot(page, dir, `refused-${lang}-${vpName}-2-after-click`, events, { clicked, lang, viewport: vpName, ...facts });
        await page.close();
        await prep('php-state', events);
    }
    await prep('broken-remove', events);
    await prep('end', events);
    // 2. the Domains row while the pool was not read, then after the page read it again
    await prep('u-unread', events);
    for (const [lang, vpName, viewport] of [['en', 'desktop', DESKTOP], ['tr', 'desktop', DESKTOP], ['en', 'phone', PHONE], ['tr', 'phone', PHONE]]) {
        const page = await newPage(dir, `unread-${lang}-${vpName}`, { lang, viewport });
        await page.goto(base + '/domains', { waitUntil: 'networkidle2', timeout: 60000 });
        await pause(4000);
        const facts = await badgeFacts(page, domain);
        await viewShot(page, dir, `unread-${lang}-${vpName}-domains`, events, { lang, viewport: vpName, ...facts });
        await page.close();
    }
    await prep('rows', events);
    const page = await newPage(dir, 'reread-en', { lang: 'en' });
    await domainConfig(page, domain, 'en');
    await shot(page, dir, 'reread-en-desktop-1-config', events);
    await page.goto(base + '/domains', { waitUntil: 'networkidle2', timeout: 60000 });
    await pause(4000);
    await viewShot(page, dir, 'reread-en-desktop-2-domains', events, await badgeFacts(page, domain));
    await page.close();
    await prep('rows', events);
}

