grep -nE "opt_showconfig|showconfig|show_config" /usr/sbin/postfwd2 | head -12
L=$(grep -nE "if \(\\$opt_showconfig\)|if \(\\$opt_showconfig " /usr/sbin/postfwd2 | head -1 | cut -d: -f1); [ -n "$L" ] && sed -n "$L,$((L+12))p" /usr/sbin/postfwd2
printf 'id=X ; action=DUNNO\n' > /tmp/pf-ok.cf; /usr/sbin/postfwd2 -f /tmp/pf-ok.cf -C >/dev/null 2>&1; echo "valid one-rule file: exit $?"
printf 'id=X ; acton=DUNNO garbage(((\n' > /tmp/pf-bad.cf; /usr/sbin/postfwd2 -f /tmp/pf-bad.cf -C >/tmp/pfb.out 2>&1; echo "broken file: exit $?"; head -c 300 /tmp/pfb.out; echo
perl -e 'require Net::Server::Multiplex; print "load ok\n"' 2>&1 | head -2; echo "perl load exit $?"
