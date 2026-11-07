// alpha.114 lab: owner-path screenshots with the Arch chromium package over CDP (puppeteer-core).
// usage: node shot.js JOB.json   (lab host only; the panel's self-signed leaf is accepted, lab only)
// JOB: {base, username, password_file, outdir, prefix, langs:["en","tr"], widths:["desktop","phone"],
//       login: true, items:[{name, path, checkpoint?:{revision,step,draft}, actions?:[...], wait?:ms, full?:bool}]}
// actions: {click:sel} {clickText:[sel,text]} {type:[sel,text]} {wait:ms} {waitFor:sel} {eval:js}
const fs = require('fs');
const path = require('path');
const puppeteer = require('/var/tmp/a114r6/shots-tool/node_modules/puppeteer-core');

const VIEW = { desktop: { width: 1440, height: 900 }, phone: { width: 390, height: 844, isMobile: true, hasTouch: true, deviceScaleFactor: 2 } };
const sleep = ms => new Promise(r => setTimeout(r, ms));

async function grow(page, width) {
  const extra = await page.evaluate(() => {
    let best = 0;
    for (const el of document.querySelectorAll('*')) {
      const cs = getComputedStyle(el);
      if (/(auto|scroll)/.test(cs.overflowY) && el.scrollHeight > el.clientHeight + 4) best = Math.max(best, el.scrollHeight - el.clientHeight);
    }
    return Math.min(best, 12000);
  });
  return extra;
}

async function act(page, a, ctx) {
  if (a.shot) {
    const v = VIEW[ctx.width]; const extra = await grow(page, ctx.width);
    if (extra > 0) await page.setViewport({ ...v, height: v.height + extra });
    await sleep(400);
    const f = ctx.file.replace(/-(en|tr)-(desktop|phone)\.png$/, `-${a.shot}-$1-$2.png`);
    await page.screenshot({ path: f, fullPage: true });
    if (extra > 0) await page.setViewport(v);
    fs.writeFileSync(f.replace(/\.png$/, '.txt'), await page.evaluate(() => document.body.innerText.slice(0, 6000)));
    const ov = await page.evaluate(() => ({ sw: Math.max(document.documentElement.scrollWidth, document.body.scrollWidth), iw: window.innerWidth }));
    ctx.results.push({ name: ctx.name + '-' + a.shot, lang: ctx.lang, width: ctx.width, file: path.basename(f), sw: ov.sw, iw: ov.iw, reloads: ctx.navs() });
    return;
  }
  if (a.waitText) {
    const [text, ms] = a.waitText; const t0 = Date.now(); let ok = false;
    while (Date.now() - t0 < (ms || 60000)) {
      ok = await page.evaluate(t => t.split('|').some(x => document.body.innerText.includes(x)), text);
      if (ok) break; await sleep(1000);
    }
    ctx.results.push({ name: ctx.name, lang: ctx.lang, width: ctx.width, waitText: text, found: ok, seconds: Math.round((Date.now() - t0) / 1000), reloads: ctx.navs() });
    return;
  }
  if (a.waitGone) {
    const [text, ms] = a.waitGone; const t0 = Date.now(); let gone = false;
    while (Date.now() - t0 < (ms || 60000)) {
      gone = !(await page.evaluate(t => document.body.innerText.includes(t), text));
      if (gone) break; await sleep(1000);
    }
    ctx.results.push({ name: ctx.name, lang: ctx.lang, width: ctx.width, waitGone: text, gone, seconds: Math.round((Date.now() - t0) / 1000), reloads: ctx.navs() });
    return;
  }
  if (a.waitJs) {
    const [js, ms] = a.waitJs; const t0 = Date.now(); let ok = false;
    while (Date.now() - t0 < (ms || 60000)) {
      try { ok = !!(await page.evaluate(js)); } catch (e) { ok = false; }
      if (ok) break; await sleep(1000);
    }
    ctx.results.push({ name: ctx.name, lang: ctx.lang, width: ctx.width, waitJs: js.slice(0, 200), ok, seconds: Math.round((Date.now() - t0) / 1000), reloads: ctx.navs() });
    return;
  }
  if (a.probe) {
    let v; try { v = await page.evaluate(a.probe); } catch (e) { v = 'probe error: ' + String(e).slice(0, 200); }
    ctx.results.push({ name: ctx.name, lang: ctx.lang, width: ctx.width, probe: a.label || '', value: v, reloads: ctx.navs() });
    return;
  }
  if (a.rewrite !== undefined) { ctx.page.__rewrite = a.rewrite; ctx.results.push({ name: ctx.name, lang: ctx.lang, width: ctx.width, probe: "rewrite", value: JSON.stringify(a.rewrite && a.rewrite.url) }); return; }
  if (a.clickIf) { await page.evaluate(sel => { const e = document.querySelector(sel); if (e) e.click(); }, a.clickIf); return; }
  if (a.click) { await page.waitForSelector(a.click, { timeout: 15000 }); await page.click(a.click); }
  else if (a.clickText) {
    const [sel, text] = a.clickText;
    const ok = await page.evaluate((sel, text) => {
      const alts = text.split('|'); const el = [...document.querySelectorAll(sel)].find(e => { const t = (e.innerText || e.textContent || '').trim(); return alts.some(a => t === a) ; }) || [...document.querySelectorAll(sel)].find(e => { const t = (e.innerText || e.textContent || '').trim(); return alts.some(a => t.includes(a)); });
      if (el) { el.click(); return true; } return false;
    }, sel, text);
    if (!ok) throw new Error('no element ' + sel + ' with text ' + text);
  }
  else if (a.type) { await page.waitForSelector(a.type[0], { timeout: 15000 }); await page.click(a.type[0], { clickCount: 3 }); await page.type(a.type[0], a.type[1]); }
  else if (a.wait) await sleep(a.wait);
  else if (a.waitFor) await page.waitForSelector(a.waitFor, { timeout: a.timeout || 20000 });
  else if (a.eval) await page.evaluate(a.eval);
}

async function settle(page, extra) {
  try { await page.waitForNetworkIdle({ idleTime: 700, timeout: 15000 }); } catch (e) { /* polling screens never idle */ }
  await sleep(extra || 1200);
}

(async () => {
  const job = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
  const password = job.password_file ? JSON.parse(fs.readFileSync(job.password_file, 'utf8')).password : '';
  fs.mkdirSync(job.outdir, { recursive: true });
  const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', headless: true, acceptInsecureCerts: true, protocolTimeout: 60000,
    args: ['--no-sandbox', '--ignore-certificate-errors', '--disable-gpu', '--no-first-run', '--disable-dev-shm-usage'] });
  const results = [];
  try {
    for (const width of job.widths || ['desktop', 'phone']) {
      const ctx = await browser.createBrowserContext();
      const page = await ctx.newPage();
      await page.setViewport(VIEW[width]);
      page.__rewrite = null; await page.setRequestInterception(true);
      page.on('request', req => { const rw = page.__rewrite; try { if (rw && req.method() !== 'GET' && new RegExp(rw.url).test(req.url())) { const b = JSON.parse(req.postData() || '{}'); Object.assign(b, rw.set); results.push({ width, rewritten: req.method() + ' ' + req.url().replace(job.base, ''), fields: Object.keys(rw.set) }); return req.continue({ postData: JSON.stringify(b) }); } } catch (e) {} req.continue(); });
      try { await ctx.overridePermissions(job.base, ['clipboard-read', 'clipboard-write', 'clipboard-sanitized-write']); } catch (e) {}
      page.on('pageerror', e => results.push({ width, pageerror: String(e).slice(0, 300) }));
      // run 3: confirm() dialogs are accepted as the owner clicks OK; DELETE answers of the domains API are recorded.
      page.on('dialog', d => { results.push({ width, dialog: d.message().slice(0, 300) }); d.accept().catch(() => {}); });
      page.on('response', async r => { try { const m = r.request().method(); if (m !== 'GET' && /\/api\/v1\//.test(r.url()) && !/\/auth\/login/.test(r.url())) results.push({ width, apiDelete: m + ' ' + r.url().replace(job.base, ''), status: r.status(), body: (await r.text()).replace(/"(password|token|secret)[^"]*"\s*:\s*"[^"]*"/gi, '"$1":"[REDACTED]"').slice(0, 1500), at: new Date().toISOString() }); } catch (e) {} });
      await page.goto(job.base + '/', { waitUntil: 'domcontentloaded', timeout: 45000 });
      await settle(page);
      if (job.login !== false) {
        await page.evaluate(l => localStorage.setItem('celikpanel.lang', l), (job.langs || ['en'])[0]);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await settle(page);
        if (job.shotLogin) await page.screenshot({ path: path.join(job.outdir, `${job.prefix}-login-${(job.langs || ['en'])[0]}-${width}.png`), fullPage: true });
        const hasForm = await page.$('#username');
        if (hasForm) {
          await page.type('#username', job.username); await page.type('#password', password);
          await Promise.all([page.click('button[type=submit]'), sleep(2500)]);
          await settle(page);
        }
      }
      for (const lang of job.langs || ['en', 'tr']) {
        for (const item of job.items) {
          const file = path.join(job.outdir, `${job.prefix}-${item.name}-${lang}-${width}.png`);
          try {
            await page.evaluate((l, cp, key) => {
              localStorage.setItem('celikpanel.lang', l);
              if (cp) sessionStorage.setItem(key, JSON.stringify(cp));
            }, lang, item.checkpoint ? { version: 1, ...item.checkpoint } : null, 'celikpanel.setup.editor.' + job.username);
            await page.goto(job.base + item.path, { waitUntil: 'domcontentloaded', timeout: 45000 });
            await settle(page, item.wait);
            let navs = 0; const onNav = fr => { if (fr === page.mainFrame()) navs++; }; page.on('framenavigated', onNav);
            const ctx = { width, lang, file, name: item.name, results, navs: () => navs, page };
            page.setDefaultTimeout(30000);
            for (const a of item.actions || []) { await act(page, a, ctx); if (!a.shot && !a.waitText && !a.probe && !a.waitJs) await settle(page, a.after || 600); }
            page.off('framenavigated', onNav);
            // The panel scrolls an inner container, not the body: grow the viewport to its full height.
            const extra = await page.evaluate(() => {
              let best = 0;
              for (const el of document.querySelectorAll('*')) {
                const cs = getComputedStyle(el);
                if (/(auto|scroll)/.test(cs.overflowY) && el.scrollHeight > el.clientHeight + 4) best = Math.max(best, el.scrollHeight - el.clientHeight);
              }
              return Math.min(best, 12000);
            });
            const v = VIEW[width];
            if (extra > 0 && item.full !== false) await page.setViewport({ ...v, height: v.height + extra });
            await sleep(400);
            await page.screenshot({ path: file, fullPage: item.full !== false });
            if (extra > 0) await page.setViewport(v);
            const text = await page.evaluate(() => document.body.innerText.slice(0, 6000));
            fs.writeFileSync(file.replace(/\.png$/, '.txt'), text);
            const ov = await page.evaluate(() => ({ sw: Math.max(document.documentElement.scrollWidth, document.body.scrollWidth), iw: window.innerWidth }));
            results.push({ name: item.name, lang, width, sw: ov.sw, iw: ov.iw, file: path.basename(file), url: page.url().replace(job.base, '').replace(/#t=[REDACTED] '#t=[REDACTED]') });
          } catch (e) {
            results.push({ name: item.name, lang, width, error: String(e).slice(0, 300) });
          }
        }
      }
      await ctx.close();
    }
  } finally { await browser.close(); }
  fs.appendFileSync(path.join(job.outdir, 'shots.jsonl'), results.map(r => JSON.stringify({ prefix: job.prefix, ...r })).join('\n') + '\n');
  console.log(JSON.stringify({ shots: results.filter(r => r.file).length, errors: results.filter(r => r.error || r.pageerror), wide: results.filter(r => r.sw && r.iw && r.sw > r.iw + 1).map(r => `${r.name}/${r.lang}/${r.width} ${r.sw}>${r.iw}`), dialogs: results.filter(r => r.dialog), deletes: results.filter(r => r.apiDelete || r.rewritten), probes: results.filter(r => r.probe !== undefined || r.waitText || r.waitGone || r.waitJs).map(r => ({ n: r.name, l: r.lang, w: r.width, p: r.probe, v: r.value, wt: r.waitText || r.waitGone || r.waitJs, ok: r.found ?? r.gone ?? r.ok, s: r.seconds, reloads: r.reloads })) }));
})();
