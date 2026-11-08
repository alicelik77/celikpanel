cat /var/tmp/a130o/logs/$1 2>/dev/null | cut -c1-${2:-600}
