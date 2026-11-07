# guest (root, Debian 12, lab only): postfwd 1.35 from Debian. Does a reload reach the policy children? Does a
# restart with --keep_rates --save_rates take new rules and keep the hourly counters? usage: g_pf135.sh [drop]
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin DEBIAN_FRONTEND=noninteractive
set -u
if ! dpkg -s postfwd >/dev/null 2>&1; then apt-get update -q >/dev/null 2>&1; apt-get install -y -q postfwd >/dev/null 2>&1; fi
dpkg -l postfwd | tail -1; grep -v '^#' /etc/default/postfwd | grep -v '^$'
mkdir -p /etc/postfix/cp
ask() { # ask USER N: one END-OF-MESSAGE request with one recipient; prints postfwd's action
  exec 3<>/dev/tcp/127.0.0.1/10040 || { echo "connection refused"; return; }
  printf 'request=smtpd_access_policy\nprotocol_state=END-OF-MESSAGE\nprotocol_name=ESMTP\nclient_address=192.0.2.9\nclient_name=lab\nhelo_name=lab\nsender=%s\nrecipient=r%s@y.test\nrecipient_count=1\nsasl_username=%s\nsize=100\ninstance=lab%s.%s\n\n' "$1" "$2" "$1" "$2" "$(date +%s%N)" >&3
  local line; IFS= read -r -t 10 line <&3; exec 3<&- 3>&-; echo "$line"
}
rules() { # rules LIMIT: a@x.test in mailbox-LIMIT.list with its rule
  rm -f /etc/postfix/cp/*.list; echo a@x.test > /etc/postfix/cp/mailbox-$1.list
  printf 'id=CP_MAILBOX_%s ; protocol_state==END-OF-MESSAGE ; sasl_username==file:/etc/postfix/cp/mailbox-%s.list ; action=rcpt(sasl_username/%s/3600/450 4.7.1 limit %s)\nid=CP_DEFAULT ; action=DUNNO\n' "$1" "$1" "$1" "$1" > /etc/postfix/postfwd.cf
  chmod 0644 /etc/postfix/postfwd.cf /etc/postfix/cp/*.list
}
mkdir -p /etc/systemd/system/postfwd.service.d
if [ "${1:-}" = old ]; then
  printf '[Service]\nExecStart=\nExecStart=/usr/sbin/postfwd $ARGS --cache=0 --keep_rates --daemon --file $CONF --interface $INET --port $PORT --user $RUNAS --group $RUNAS --pidfile $PIDFILE\n' > /etc/systemd/system/postfwd.service.d/celikpanel.conf
else
  printf '[Service]\nExecStartPre=+/usr/bin/install -d -o ${RUNAS} -g ${RUNAS} -m 0700 /var/lib/celikpanel-postfwd\nExecStart=\nExecStart=/usr/sbin/postfwd $ARGS --cache=0 --keep_rates --save_rates /var/lib/celikpanel-postfwd/rates --daemon --file $CONF --interface $INET --port $PORT --user $RUNAS --group $RUNAS --pidfile $PIDFILE\n' > /etc/systemd/system/postfwd.service.d/celikpanel.conf
fi
cat /etc/systemd/system/postfwd.service.d/celikpanel.conf
rules 2; systemctl daemon-reload; systemctl restart postfwd; sleep 2; systemctl is-active postfwd
echo "== A limit 2: three messages"; for i in 1 2 3; do echo " $i: $(ask a@x.test $i)"; done
echo "== B rules changed to limit 5, systemctl reload"; rules 5; T=$(date +%s.%N); systemctl reload postfwd; sleep 2
for i in 4 5 6; do echo " $i: $(ask a@x.test $i)"; done
echo "   second reload (the lab's next change):"; systemctl reload postfwd; sleep 2
for i in 7 8; do echo " $i: $(ask a@x.test $i)"; done
journalctl -u postfwd --since "@${T%.*}" -o cat --no-pager | grep -E "HUP|reload|respawn|uninitialized|closing|hits=" | sed -E 's/.*(hits=[^,]*).*(action=.*)/  \1 \2/' | cut -c1-160
echo "== C limit 5 kept, systemctl restart: does the counter survive (save_rates)?"
ls -la /var/lib/celikpanel-postfwd 2>&1 | tail -2
T0=$(date +%s%N); systemctl restart postfwd; T1=$(date +%s%N); echo "   restart took $(( (T1-T0)/1000000 )) ms"
for w in 1 2 3 4 5 6 7 8 9 10; do a=$(ask a@x.test probe$w); [ "$a" != "connection refused" ] && break; sleep 0.5; done; echo "   first answer after restart: $a"
ls -la /var/lib/celikpanel-postfwd 2>&1 | tail -2
for i in 9 10 11; do echo " $i: $(ask a@x.test $i)"; done
echo "== D rules changed to limit 3, systemctl restart + --delrate for the changed mailbox"
rules 3; systemctl restart postfwd; sleep 2; /usr/sbin/postfwd --delrate "sasl_username=a@x.test" 2>&1 | tail -1
for i in 12 13 14 15; do echo " $i: $(ask a@x.test $i)"; done
journalctl -u postfwd --since -1min -o cat --no-pager | grep -E "hits=|rates|ERROR|Could not" | sed -E 's/.*(hits=[^,]*).*(action=.*)/  \1 \2/' | cut -c1-160 | tail -8
systemctl is-active postfwd
