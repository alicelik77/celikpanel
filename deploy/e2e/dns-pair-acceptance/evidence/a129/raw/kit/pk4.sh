grep -E "^==|msg|<~\*|<~  250|paused|applied" /var/tmp/a129o/logs/C4mail-d12.log 2>/dev/null | grep -v 235 | tail -30 | cut -c1-200
