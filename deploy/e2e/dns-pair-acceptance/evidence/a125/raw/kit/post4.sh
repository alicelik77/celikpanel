# alpha.125 cases 4 (mail phase 2) and 5 (typed 409s) on the updated main server (Debian 13, mail). Lab only.
export CELL=p5 IP=192.0.3.190 Z=p5.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C45-$CELL.log) 2>&1
SID=$(cat $T/sid); MID=$(cat $T/mid); read SU MU < $T/users
pwrep() { adm GET /api/v1/mail/passwordless --label passwordless --timeout 90 | sbrief ${1:-1600}; }
echo "== 4.0 Postfix settings and record after the update $(ts)"
gshn main 'postconf authorized_submit_users smtpd_relay_restrictions mynetworks; python3 -c "import json; d=json.load(open(\"/var/lib/celikpanel-agent-private/mail-passwordless.json\")); print({k:d.get(k) for k in d if k not in (\"cron_mailto_changed\",)}); print(\"cron changed:\", d.get(\"cron_mailto_changed\"))"'
pwrep
echo "== 4.1 site crontabs: shop5.test (panel-written before the update) got MAILTO=\"\" once; $Z (owner MAILTO) kept $(ts)"
grun main g_cron.sh show $SU $MU
echo "-- the panel writes the owner's crontab again (adds a job): the owner's MAILTO stays"
cronadd $MID "45 4 * * *" "/bin/true # a125 second job"
echo "-- the panel writes shop5.test's crontab again: one MAILTO only"
cronadd $SID "5 5 * * *" "/bin/true # a125 shop job"
grun main g_cron.sh show $SU $MU
echo "== 4.2 a mailbox on another lab server (d5.test, Debian 12) to receive: ext5@d5.test, acct5@d5.test $(ts)"
DID5=$(CELL=d5 domain_id d5.test); echo "d5.test id=$DID5"
for a in ext5 acct5; do b1 api d5 main POST /api/v1/domains/$DID5/mail/accounts "{\"address\":\"$a@d5.test\",\"password\":\"$(python3 -c 'import secrets; print(secrets.token_urlsafe(18)+"a1A")')\",\"quota_mb\":50}" --label mbx-$a --limit 3000 | python3 -c 'import json,sys; d=json.load(sys.stdin); print("  create", sys.argv[1], d.get("status"))' $a; done
echo "== 4.3 the sending paths on the main server $(ts)"
read SDR SU2 < <(siteof shop5.test); echo "shop5.test $SDR $SU2"
grun main g_mail5.sh shop5.test $SDR $SU2 inbox5@$Z ext5@d5.test
echo "-- 587 with the mailbox password (noreply@shop5.test -> ext5@d5.test)"
gput g_mail587.sh main; pw mbx5 | timeout 120 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/g_mail587.sh noreply@shop5.test ext5@d5.test"
echo "== 4.4 panel account mail (Agent.SendAccountMailV1): the customer changes their address to acct5@d5.test $(ts)"
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"email":"acct5@d5.test","current_password":c["password"]},open(sys.argv[2],"w"))' $T/cred-cust5.json $T/em.json
as cust5 PUT /api/v1/auth/email @$T/em.json | sbrief 400; rm -f $T/em.json
sleep 20
gshn main 'journalctl -q -o short-iso --since -2min --no-pager | grep -E "uid=0 from=|to=<acct5@d5.test>|to=<cust5|SendAccountMail|account mail" | cut -c1-240 | tail -8'
echo "== 4.5 what reached the other server (d5) $(ts)"
sleep 30
for a in ext5@d5.test acct5@d5.test; do echo "-- $a"; CELL=d5 grun main g_maildir.sh $a; done
gshn main 'postqueue -p | tail -5'
echo "== 4.6 journal refusals on the main server $(ts)"
gshn main 'journalctl -q -o short-iso --since -15min --no-pager -t postfix/sendmail -t postfix/postdrop -t postfix/smtpd | grep -E "not allowed to submit|Relay access denied" | cut -c1-240'
pwrep
echo "== 5.1 a mailbox that already exists: inbox5@$Z again $(ts)"
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"address":"inbox5@'$Z'","password":c["password"],"quota_mb":50},open(sys.argv[2],"w"))' $T/cred-mbx5in.json $T/m.json
adm POST /api/v1/domains/$MID/mail/accounts @$T/m.json --label create-inbox-again | sbrief 900; rm -f $T/m.json
echo "== 5.2 PHP version and PHP pool of the static main site $Z $(ts)"
adm GET /api/v1/domains/$MID/php --label php-read | sbrief 600
adm POST /api/v1/domains/$MID/php '{"php_version":"8.4"}' --label php-change | sbrief 900
adm GET /api/v1/domains/$MID/php/pool --label pool-read | sbrief 900
adm POST /api/v1/domains/$MID/php/pool '{"pm":"ondemand","max_children":5}' --label pool-save | sbrief 900
echo "-- the PHP site shop5.test still answers (customer)"; as cust5 GET /api/v1/domains/$SID/php | sbrief 300; as cust5 GET /api/v1/domains/$SID/php/pool | sbrief 300
echo "-- the English and Turkish sentences in the served web bundle"
gshn main 'W=$(dirname $(find /opt/celikpanel /usr/share/celikpanel -name index.html -path "*dist*" 2>/dev/null | head -1)); echo "web $W"; for s in "is a static site: it serves files" "statik bir site: dosyaları olduğu gibi" "already exists, so nothing was changed" "posta kutusu zaten var" "the update waits for it (up to a minute)" "güncelleme onu bekler (en çok bir dakika)"; do printf "  %-48s %s file(s)\n" "$s" "$(grep -rlF "$s" $W/assets 2>/dev/null | wc -l)"; done'
echo "== 4.7 the owner sets their own values, then restarts the panel (systemctl restart celikpanel-panel) $(ts)"
gshn main 'postconf -e "authorized_submit_users = static:anyone" "smtpd_relay_restrictions = permit_mynetworks, permit_sasl_authenticated, defer_unauth_destination"; postfix reload 2>&1 | tail -1; systemctl restart celikpanel-panel; echo restarted'
sleep 90; b1 login $CELL main > /dev/null 2>&1
gshn main 'postconf authorized_submit_users smtpd_relay_restrictions'
pwrep 2500
gshn main 'journalctl -u celikpanel-panel --since -3min -o cat --no-pager | grep -iE "passwordless|authorized_submit|owner" | cut -c1-300 | tail -6'
echo "== post4 done $(ts)"
