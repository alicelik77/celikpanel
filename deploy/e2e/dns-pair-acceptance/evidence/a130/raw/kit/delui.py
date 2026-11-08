# delui.py DOMAIN OUTFILE: shot.js items for deleting DOMAIN from the Domains page as the owner does
# (the panel's own confirm window, Delete pressed twice fast, the progress window followed to its end). Lab only.
import json, sys
d, out = sys.argv[1], sys.argv[2]
observer = """(() => { window.__log = []; window.__t0 = Date.now(); window.__last = '';
 const rec = () => { const st = [...document.querySelectorAll('[data-operation-step]')].map(e => e.dataset.operationStep + ':' + e.dataset.stepState).join(' ');
   const end = document.querySelector('[data-operation-close]') ? 'END' : ''; const s = st + ' ' + end;
   if (s !== window.__last) { window.__last = s; window.__log.push(((Date.now() - window.__t0) / 1000).toFixed(1) + 's ' + s); } };
 new MutationObserver(rec).observe(document.body, { subtree: true, childList: true, attributes: true }); })()"""
click_delete = """(() => { const tr = [...document.querySelectorAll('tr')].find(r => r.innerText.includes('%s')); const bs = [...tr.querySelectorAll('button')]; bs[bs.length - 1].click(); })()""" % d
double = """(() => { const b = document.getElementById('confirm-action-confirm'); b.click(); b.click(); setTimeout(() => { const c = document.getElementById('confirm-action-confirm'); if (c) c.click(); }, 120); })()"""
lock = """JSON.stringify({ dialogs: document.querySelectorAll('[role=dialog],[role=alertdialog]').length, inert: [...document.querySelectorAll('[inert]')].length, running: !!document.querySelector('[data-operation-running]'), steps: [...document.querySelectorAll('[data-operation-step]')].map(e => e.dataset.operationStep + ':' + e.dataset.stepState + ' ' + e.innerText.replace(/\s+/g, ' ').slice(0, 120)) })"""
items = [{"name": "del-" + d.split(".")[0], "path": "/domains", "wait": 2500, "actions": [
    {"eval": observer},
    {"eval": click_delete},
    {"waitFor": "#confirm-action-confirm"},
    {"probe": "document.getElementById('confirm-action-confirm').closest('[role=dialog],[role=alertdialog]') ? document.getElementById('confirm-action-confirm').closest('[role=dialog],[role=alertdialog]').innerText.slice(0, 400) : 'no dialog'", "label": "confirm-window"},
    {"shot": "confirm"},
    {"eval": double, "after": 300},
    {"probe": lock, "label": "progress-locked"},
    {"shot": "progress"},
    {"waitJs": ["[...document.querySelectorAll('[data-operation-step]')].some(e => e.dataset.stepState === 'running' && /dns/i.test(e.dataset.operationStep))", 300000]},
    {"probe": lock, "label": "progress-dns"},
    {"shot": "progress-dns"},
    {"waitJs": ["!!document.querySelector('[data-operation-close]')", 900000]},
    {"probe": "document.querySelector('#operation-progress') ? document.querySelector('#operation-progress').innerText.slice(0, 600) : (document.querySelector('[role=dialog]') || document.body).innerText.slice(0, 600)", "label": "end-window"},
    {"shot": "end"},
    {"probe": "JSON.stringify(window.__log)", "label": "step-log"},
    {"click": "[data-operation-close]", "after": 4000},
    {"probe": "[...document.querySelectorAll('tr')].some(r => r.innerText.includes('%s')) ? 'row still listed' : 'row gone without reload'" % d, "label": "list-after"},
]}]
json.dump(items, open(out, "w"))
