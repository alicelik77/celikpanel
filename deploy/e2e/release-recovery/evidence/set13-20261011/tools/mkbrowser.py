import sys
root = sys.argv[1]
src = open(root + '/cold-load-set12.mjs', encoding='utf-8').read()
head_end = src.index('async function takePool')
tail_start = src.index('const results = {};')
head = src[:head_end]
old_intro = head[:head.index('//\n//   BROWSER_INSPECT_MODULES')]
new_intro = '''// node cold-load-set13.mjs <handover-dir> <out-dir> <cell[,cell...]>
// set13 (2026-10-11): commit 816b8dd60's answer to a refused choice and the Domains row of a file that was not read, in a
// real headless Chrome against a REAL CelikPanel on a disposable lab guest, reached through the lab driver's loopback
// SSH forward (deploy/e2e/release-recovery/set13_trial.py, set12's browser window "ui"). Nothing but the loopback
// address of the forward is opened. The driver prepares each state on request (prep-request-<name>-<n> -> prep-done).
//
//   cells: ui    the refused "take" on an owner-edited pool while an owner's pool file makes php-fpm -t fail: after
//                the click, the answer ([data-site-config-refusal], role=status) and whether its box is inside the
//                viewport (EN desktop, TR phone; viewport and full-page pictures); then the Domains row while the pool
//                was not read ("could not be read", EN and TR, desktop and phone), and after the page was read again
//          done  writes done.json
'''
head = new_intro + head[len(old_intro):]
body = open(sys.argv[2], encoding='utf-8').read()
tail = src[tail_start:].replace("cold-load-set12.mjs", "cold-load-set13.mjs")
out = head.replace("usage: node cold-load-set12.mjs", "usage: node cold-load-set13.mjs") + body + tail
open(root + '/cold-load-set13.mjs', 'w', encoding='utf-8', newline='\n').write(out)
print('written', len(out))
