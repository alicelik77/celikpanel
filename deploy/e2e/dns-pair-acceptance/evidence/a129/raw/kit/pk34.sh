for e in mysql postgresql; do echo "#### $e"; grep -E "^== |TAKEN|denied|ERROR|ok|PASS|fail|intact|permission" /var/tmp/a129o/logs/C3dbu-r9-$e.log | cut -c1-220 | head -45; done
