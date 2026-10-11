#!/bin/bash
# set11: the browser on Windows. Waits for each handover directory the driver opens under /var/tmp/cp-set11-run/hand
# (ready.json) and runs cold-load-set11.mjs once for it: b-* windows -> siteconfig,done; d-e -> slow35,afterfail,
# afterstart,sw,done. Ends when browser/runner.stop exists.
S=<scratchpad>/set11
export BROWSER_INSPECT_MODULES=$(cygpath -w $S/../set7/browser-modules)
JS="<repo>/web/tools/browser-inspect/cold-load-set11.mjs"
HROOT=//wsl.localhost/archlinux/var/tmp/cp-set11-run/hand
echo "$(date -u +%FT%TZ) runner started pid=$$" >> $S/browser/runner.log
while [ ! -e $S/browser/runner.stop ]; do
  for d in $(ls "$HROOT" 2>/dev/null); do
    [ -e "$S/browser/started-$d" ] && continue
    [ -e "$HROOT/$d/ready.json" ] || continue
    touch "$S/browser/started-$d"
    case $d in *-b-*) cells=siteconfig,done;; *-d-e) cells=slow35,afterfail,afterstart,sw,done;; *) cells=done;; esac
    # set11 (after Debian run-c): from the Ubuntu lab on, cell E runs in a second browser process whose profile has a
    # short path (SHORT_PROFILE_ROOT), the other cells as before.
    if [ -n "${SHORT_PROFILE_ROOT:-}" ] && [ "$cells" = "slow35,afterfail,afterstart,sw,done" ]; then
      echo "$(date -u +%FT%TZ) start $d slow35,afterfail,afterstart" >> $S/browser/runner.log
      node "$JS" "$HROOT/$d" "$(cygpath -w $S/browser/$d)" "slow35,afterfail,afterstart" >> $S/browser/runner.log 2>&1
      echo "$(date -u +%FT%TZ) rc=$? $d (first process)" >> $S/browser/runner.log
      mkdir -p "$SHORT_PROFILE_ROOT"; cells=sw,done
      echo "$(date -u +%FT%TZ) start $d $cells profile_root=$(cygpath -w $SHORT_PROFILE_ROOT)" >> $S/browser/runner.log
      LIVE_PROFILE_ROOT=$(cygpath -w $SHORT_PROFILE_ROOT) LIVE_TAG=shortprofile node "$JS" "$HROOT/$d" "$(cygpath -w $S/browser/$d)" "$cells" >> $S/browser/runner.log 2>&1
    else
      echo "$(date -u +%FT%TZ) start $d $cells" >> $S/browser/runner.log
      node "$JS" "$HROOT/$d" "$(cygpath -w $S/browser/$d)" "$cells" >> $S/browser/runner.log 2>&1
    fi
    echo "$(date -u +%FT%TZ) rc=$? $d" >> $S/browser/runner.log
  done
  sleep 5
done
echo "$(date -u +%FT%TZ) runner stopped" >> $S/browser/runner.log
