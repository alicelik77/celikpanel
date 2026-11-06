#!/usr/bin/env python3
"""uiitems.py KIND DOMAIN OUTFILE [extra]: shot.js items for the alpha.127 UI cases (5, 6, 7). Lab only.
KIND: pw-mail | pw-db | pw-access | access-view | access-install | webmail | op-php | op-backup | op-restore | op-dns |
      op-ssl | op-service (DOMAIN = component id) | tour-probe"""
import json, sys
kind, dom, out = sys.argv[1], sys.argv[2], sys.argv[3]
extra = sys.argv[4] if len(sys.argv) > 4 else ""

OBSERVER = """(() => { window.__log = []; window.__t0 = Date.now(); window.__last = ''; window.__tourDuring = 0;
 const rec = () => { const host = document.querySelector('[data-confirm-host]');
   const st = [...document.querySelectorAll('[data-operation-step]')].map(e => e.dataset.operationStep + ':' + e.dataset.stepState).join(' ');
   const tour = document.querySelector('[data-tour-overlay],[data-tour-offer]') ? ' TOUR' : '';
   if (host && tour) window.__tourDuring++;
   const end = (document.querySelector('[data-operation-end]') || {dataset: {}}).dataset.operationEnd || '';
   const s = (host ? host.dataset.confirmHost : '-') + ' ' + st + ' ' + end + tour;
   if (s !== window.__last) { window.__last = s; window.__log.push(((Date.now() - window.__t0) / 1000).toFixed(1) + 's ' + s); } };
 new MutationObserver(rec).observe(document.body, { subtree: true, childList: true, attributes: true }); rec(); })()"""
OPWIN = """JSON.stringify({ host: (document.querySelector('[data-confirm-host]') || {dataset: {}}).dataset.confirmHost || null,
 aria: !!document.querySelector('[aria-modal=true]'), running: !!document.querySelector('[data-operation-running]'),
 close: !!document.querySelector('[data-operation-close]'), end: (document.querySelector('[data-operation-end]') || {dataset: {}}).dataset.operationEnd || null,
 tour: !!document.querySelector('[data-tour-overlay],[data-tour-offer]'),
 title: ((document.querySelector('#operation-progress') || document.querySelector('#confirm-action') || {}).innerText || '').replace(/\\s+/g, ' ').slice(0, 900),
 steps: [...document.querySelectorAll('[data-operation-step]')].map(e => e.dataset.operationStep + ':' + e.dataset.stepState) })"""
LOG = "JSON.stringify({log: window.__log, tourDuringWindow: window.__tourDuring})"
TOASTS = "JSON.stringify([...document.querySelectorAll('.fixed.top-4.right-4 [role]')].map(e => e.innerText.slice(0, 200)))"

def tab(en, tr):  # a domain page tab or sub-tab (plain buttons, exact text first)
    return {"clickText": ["main button", f"{en}|{tr}"], "after": 1500}

def btn(en, tr, after=1200):
    return {"clickText": ["button", f"{en}|{tr}"], "after": after}

PWPROBE = """(() => { const inp = [...document.querySelectorAll('input[autocomplete=new-password]')].filter(e => e.offsetParent !== null)[0];
 if (!inp) return 'no new-password input';
 const box = inp.closest('.min-w-0') || inp.parentElement.parentElement;
 const scope = inp.closest('form') || inp.closest('section') || inp.closest('[role=dialog]') || document.body;
 const subs = [...scope.querySelectorAll('button')].filter(b => /^(Create|Oluştur|Create database|Veritabanı oluştur|Review the change|Değişikliği gözden geçir|Apply the change|Değişikliği uygula)$/.test(b.innerText.trim())).map(b => b.innerText.trim() + (b.disabled ? ':disabled' : ':enabled'));
 return JSON.stringify({ type: inp.type, len: inp.value.length, field: box.innerText.replace(/\\s+/g, ' ').slice(0, 500),
   buttons: [...box.querySelectorAll('button')].map(b => (b.getAttribute('aria-label') || b.innerText) + (b.getAttribute('aria-pressed') ? '[pressed=' + b.getAttribute('aria-pressed') + ']' : '')),
   submit: subs }); })()"""
def click_aria(rx):
    return {"eval": "(() => { const b = [...document.querySelectorAll('button[aria-label]')].filter(e => e.offsetParent !== null).find(e => new RegExp(%s).test(e.getAttribute('aria-label'))); if (b) b.click(); })()" % json.dumps(rx), "after": 700}
REFUSAL = """JSON.stringify({ banners: [...document.querySelectorAll('[role=alert]')].filter(e => !e.closest('.fixed.top-4.right-4')).map(e => e.innerText.replace(/\\s+/g, ' ').slice(0, 400)),
 toasts: [...document.querySelectorAll('.fixed.top-4.right-4 [role]')].map(e => e.innerText.slice(0, 200)),
 weakTextCount: (document.body.innerText.match(/Nothing was changed|Hiçbir şey değiştirilmedi/g) || []).length })"""

def pw_flow(open_actions, weak_field_url, set_name=None):
    acts = [*open_actions, {"probe": PWPROBE, "label": "empty-field (rule up front, submit off)"}, {"shot": "rule"}]
    if set_name:
        acts.append(set_name)
    acts += [{"type": ["input[autocomplete=new-password]", "abc"]}, {"probe": PWPROBE, "label": "weak typed"}, {"shot": "weak"},
             click_aria("^(Generate|Güçlü)"), {"probe": PWPROBE, "label": "generated"},
             click_aria("^(Hide|Show|Şifreyi)"), {"probe": PWPROBE, "label": "after eye press"},
             click_aria("^(Copy|Kopyala)"), {"probe": PWPROBE + " + ' clipboard-status:' + (document.body.innerText.match(/Copied|Kopyalandı|could not be copied|kopyalanamadı/) || ['none'])[0]", "label": "after copy"},
             {"shot": "generated"},
             {"rewrite": {"url": weak_field_url, "set": {"password": "[REDACTED]"}}}]
    return acts

TICK = {"eval": "(() => { const l = [...document.querySelectorAll('label')].find(e => /File access|Dosya erişimi/.test(e.innerText)); const c = l && l.querySelector('input[type=checkbox]'); if (c && !c.checked) c.click(); })()", "after": 1000}
items = []
if kind == "pw-mail":
    acts = pw_flow([tab("Mail", "E-posta"), btn("Create address", "Adres oluştur")], r"/mail/accounts$",
                   {"type": ["input[placeholder=info]", "pwtest"]})
    acts += [btn("Create", "Oluştur", 3000), {"probe": REFUSAL, "label": "server refusal (rewritten to Weakpass1)"}, {"shot": "refused"}, {"rewrite": None}]
    items = [{"name": "pw-mail", "path": f"/domains/{dom}", "wait": 2500, "actions": acts}]
elif kind == "pw-db":
    acts = pw_flow([tab("Databases", "Veritabanları"), btn("Create database", "Veritabanı oluştur")], r"/databases$",
                   {"type": ["input[placeholder=myapp]", extra or "pwtest"]})
    acts += [{"eval": "(() => { const b = [...document.querySelectorAll('form button, button[type=submit]')].filter(e => /^(Create database|Veritabanı oluştur)$/.test(e.innerText.trim())).pop(); if (b) b.click(); })()", "after": 4000},
             {"probe": REFUSAL, "label": "server refusal (rewritten to Weakpass1)"}, {"shot": "refused"}, {"rewrite": None}]
    items = [{"name": "pw-db", "path": f"/domains/{dom}", "wait": 2500, "actions": acts}]
elif kind == "pw-access":
    acts = pw_flow([tab("Access", "Erişim"),
                    {"eval": "(() => { const l = [...document.querySelectorAll('label')].find(e => /File access|Dosya erişimi/.test(e.innerText)); const c = l && l.querySelector('input[type=checkbox]'); if (c && !c.checked) c.click(); })()", "after": 800}],
                   r"/access(/preview)?$")
    acts += [btn("Review the change", "Değişikliği gözden geçir", 3000), {"probe": REFUSAL, "label": "server refusal (rewritten to Weakpass1)"}, {"shot": "refused"}, {"rewrite": None}]
    items = [{"name": "pw-access", "path": f"/domains/{dom}", "wait": 2500, "actions": acts}]
elif kind == "access-view":
    ACC = """JSON.stringify({ notice: [...document.querySelectorAll('[role=status]')].map(e => e.innerText.replace(/\\s+/g, ' ').slice(0, 600)),
      install: [...document.querySelectorAll('button')].some(b => /^(Install the tools|Araçları kur)$/.test(b.innerText.trim())),
      ssh: [...document.querySelectorAll('label')].some(e => /Command line \\(SSH\\)|Komut satırı \\(SSH\\)/.test(e.innerText)),
      text: (document.querySelector('main') || document.body).innerText.replace(/\\s+/g, ' ').slice(0, 1500) })"""
    items = [{"name": "access-" + extra, "path": f"/domains/{dom}", "wait": 2500, "actions": [tab("Access", "Erişim"), {"wait": 2500}, TICK, {"probe": ACC, "label": "access tab (file access ticked)"}]}]
elif kind == "access-install":
    ACC = """JSON.stringify({ install: [...document.querySelectorAll('button')].some(b => /^(Install the tools|Araçları kur)$/.test(b.innerText.trim())),
      ssh: [...document.querySelectorAll('label')].some(e => /Command line \\(SSH\\)|Komut satırı \\(SSH\\)/.test(e.innerText)) })"""
    items = [{"name": "access-install", "path": f"/domains/{dom}", "wait": 2500, "actions": [
        {"eval": OBSERVER}, tab("Access", "Erişim"), {"wait": 2000}, TICK, {"probe": ACC, "label": "before"}, {"shot": "before"},
        btn("Install the tools", "Araçları kur", 800), {"waitFor": "#confirm-action-confirm"}, {"probe": OPWIN, "label": "confirm window"}, {"shot": "confirm"},
        {"click": "#confirm-action-confirm", "after": 300}, {"probe": OPWIN, "label": "progress (locked)"}, {"shot": "progress"},
        {"waitJs": ["!!document.querySelector('[data-operation-close]')", 300000]}, {"probe": OPWIN, "label": "end"}, {"shot": "end"},
        {"probe": LOG, "label": "step-log"}, {"click": "[data-operation-close]", "after": 2500}, {"probe": ACC, "label": "after OK, no reload"}]}]
elif kind == "webmail":
    WM = """JSON.stringify([...document.querySelectorAll('[data-webmail-access]')].map(e => ({ missing: e.dataset.webmailMissing || null, text: e.innerText.replace(/\\s+/g, ' ').slice(0, 700), links: [...e.querySelectorAll('a,button')].map(a => a.innerText.trim() + ' -> ' + (a.getAttribute('href') || '')) })))"""
    items = [{"name": "webmail-" + extra, "path": f"/domains/{dom}", "wait": 2500, "actions": [tab("Mail", "E-posta"), {"wait": 3000}, {"probe": WM, "label": "webmail card"}]}]
elif kind.startswith("op-"):
    pre = {"op-php": [tab("Hosting", "Barındırma"), tab("PHP", "PHP"),
                      {"eval": "(() => { const s = [...document.querySelectorAll('select')].find(x => [...x.options].some(o => /^\\d+\\.\\d+$/.test(o.value))); const o = [...s.options].find(o => /^\\d+\\.\\d+$/.test(o.value) && o.value !== s.value && !o.disabled); const set = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set; set.call(s, o.value); s.dispatchEvent(new Event('change', { bubbles: true })); window.__picked = s.value; })()", "after": 800},
                      {"probe": "'picked ' + window.__picked", "label": "version picked"}, btn("Apply", "Uygula", 600)],
           "op-backup": [tab("Advanced", "Gelişmiş"), tab("Backups", "Yedekler"), {"wait": 1500}, {"eval": "(() => { const b = [...document.querySelectorAll('main button')].find(e => /^(Files backup|Dosya yedeği)/.test(e.innerText.trim())); if (b) b.click(); })()", "after": 400}],
           "op-restore": [tab("Advanced", "Gelişmiş"), tab("Backups", "Yedekler"), {"wait": 2000},
                          {"eval": "(() => { const b = [...document.querySelectorAll('button')].filter(e => /^(Restore|Geri yükle)$/.test(e.innerText.trim()) || /^(Restore|Geri yükle)/.test(e.getAttribute('aria-label') || '')); if (b.length) b[0].click(); })()", "after": 800}],
           "op-dns": [tab("DNS", "DNS"), btn("Add record", "Kayıt ekle", 800),
                      {"type": ["input[placeholder='@ for root'], input[placeholder='kök için @']", extra or "labrec"]},
                      {"type": ["input[placeholder='192.168.1.1']", "192.0.2.77"]}, btn("Save record", "Kaydı kaydet", 400)],
           "op-ssl": [tab("Hosting", "Barındırma"), tab("SSL/TLS", "SSL/TLS"), {"wait": 2500}, {"type": ["input[placeholder='admin@example.com']", "alerts@r7.test"]}, btn("Get certificate", "Sertifika al", 600)],
           "op-service": [{"wait": 2500}, btn("Restart", "Yeniden başlat", 600)]}[kind]
    path = f"/components/{dom}" if kind == "op-service" else f"/domains/{dom}"
    acts = [{"eval": OBSERVER}, *pre,
            {"eval": "(() => { window.__t1 = Date.now(); })()"},
            {"probe": "document.querySelector('#confirm-action-confirm') ? 'question: ' + document.querySelector('#confirm-action').innerText.replace(/\\s+/g, ' ').slice(0, 500) : 'no question (window opens directly)'", "label": "question"},
            {"probe": OPWIN, "label": "window before confirm"}, {"shot": "question"},
            {"eval": "(() => { const b = document.getElementById('confirm-action-confirm'); if (b) { b.click(); b.click(); } })()", "after": 400},
            {"probe": OPWIN, "label": "locked window (confirm pressed twice)"}, {"shot": "progress"},
            {"waitJs": ["!!document.querySelector('[data-operation-close]')", 300000]},
            {"probe": OPWIN, "label": "end"}, {"shot": "end"}, {"probe": LOG, "label": "step-log"}, {"probe": TOASTS, "label": "toasts"},
            {"click": "[data-operation-close]", "after": 3000}, {"probe": "JSON.stringify({tour: !!document.querySelector('[data-tour-overlay],[data-tour-offer]')})", "label": "tour after the window closed"}]
    items = [{"name": kind, "path": path, "wait": 2500, "actions": acts}]
json.dump(items, open(out, "w"))
print(out, len(items))
