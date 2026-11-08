for s in u1 d1; do echo "## $s"; tail -n 4 /var/tmp/a130o/logs/setup129-$s.log 2>&1 | cut -c1-220; done
