grep -hE "== |s 200|schema|differences|failed end|^>|^<" /var/tmp/a130o/logs/C1upd-r9.log | cut -c1-200 | head -30; ls /var/tmp/a130o/logs | head -80
