#!/bin/bash
# set13: the browser on Windows. Waits for each handover directory the driver opens under /var/tmp/cp-set13-run/hand
# (ready.json) and runs cold-load-set13.mjs once for it (cells ui,done). The profile goes under %TEMP%\cpb\<8 hex>
# (profile-dir.mjs; its length is recorded). Ends when browser/runner.stop exists.
S=<scratchpad>/set13
export BROWSER_INSPECT_MODULES=$(cygpath -w $S/../set7/browser-modules)
JS="<repo>/web/tools/browser-inspect/cold-load-set13.mjs"
HROOT=//wsl.localhost/archlinux/var/tmp/cp-set13-run/hand
mkdir -p $S/browser
echo "$(date -u +%FT%TZ) runner started pid=$$" >> $S/browser/runner.log
while [ ! -e $S/browser/runner.stop ]; do
  for d in $(ls "$HROOT" 2>/dev/null); do
    [ -e "$S/browser/started-$d" ] && continue
    [ -e "$HROOT/$d/ready.json" ] || continue
    touch "$S/browser/started-$d"
    echo "$(date -u +%FT%TZ) start $d ui,done" >> $S/browser/runner.log
    node "$JS" "$HROOT/$d" "$(cygpath -w $S/browser/$d)" "ui,done" >> $S/browser/runner.log 2>&1
    echo "$(date -u +%FT%TZ) rc=$? $d" >> $S/browser/runner.log
  done
  sleep 5
done
echo "$(date -u +%FT%TZ) runner stopped" >> $S/browser/runner.log
