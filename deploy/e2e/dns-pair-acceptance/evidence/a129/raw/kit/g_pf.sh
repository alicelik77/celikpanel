export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
echo "--- postfwd -C on the panel's file"; /usr/sbin/postfwd -C -f /etc/postfix/postfwd.cf > /tmp/pfc.out 2>/tmp/pfc.err; echo "exit $?"; head -c 600 /tmp/pfc.err; echo; head -5 /tmp/pfc.out
echo "--- files"; ls -la /etc/postfix/postfwd.cf /etc/postfix/celikpanel-postfwd/ 2>&1; head -30 /etc/postfix/postfwd.cf
echo "--- unit"; systemctl status postfwd --no-pager -l 2>&1 | head -20; ss -ltnp | grep 10040
echo "--- journal"; journalctl -u postfwd --since -20min --no-pager -o cat | tail -15 | cut -c1-300
readlink -f /usr/sbin/postfwd; perl -v | sed -n 2p; dpkg -l libnet-server-perl libio-multiplex-perl | tail -2
