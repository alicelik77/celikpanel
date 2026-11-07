cut -c1-330 /var/tmp/a129o/logs/C3wp-r9.log | sed -n "/18:36:15/,\$p" | sed -E "s/(password\"?: ?\")[^\"]*/REDACTED/Ig" | tail -30; echo ====; cut -c1-300 /var/tmp/a129o/logs/C4flush2-r9.log
