sed -n "/4.6b/,\$p" /var/tmp/a130o/logs/C4remote-r9-mysql.log | cut -c1-300 | uniq | tail -60; echo =====PG; cat /var/tmp/a130o/logs/C4remote-r9-postgresql.log | cut -c1-300 | uniq
