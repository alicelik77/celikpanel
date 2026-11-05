for p in $(pgrep -f 'singleX.sh|runW2b.sh'); do if grep -qa 'CELL=u24' /proc/$p/environ 2>/dev/null || grep -qa runW2b /proc/$p/cmdline; then kill $p && echo "stopped $p $(tr '\0' ' ' < /proc/$p/cmdline 2>/dev/null | cut -c1-80)"; fi; done
sleep 1; pgrep -af 'singleX|runW2b|wait_status' | cut -c1-120
