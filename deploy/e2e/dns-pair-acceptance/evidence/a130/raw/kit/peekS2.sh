for s in d12 u22; do echo "## $s"; grep -A2 "^-- " /var/tmp/a130o/logs/setup127-$s.log | tail -6 | cut -c1-400; done; date -u +%T
