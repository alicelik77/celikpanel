# alpha.130 round, item 8: mailbox recovery on A. Mailbox info@cust1 (id 2); recovery address box1@site1 (a mailbox on A,
# read by the lab as its owner would). Tokens are kept in 0600 files and never printed. usage: k8.sh PART... Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/K8recovery-$CELL.log) 2>&1
b1 login $CELL main > /dev/null 2>&1
nup n_swaks.sh > /dev/null
raw() { python3 -I $A/raw.py $L $CELL main "$@"; }
C1=$(cat $T/cust1.id); SUB=$(cat $T/sub-cust1.id); MB=2; RA=box1@site1.$Z; MBA=info@cust1.$Z
g() { grun main g_rec.sh "$@"; }
gettok() { g token $RA /root/tok.txt; timeout 60 python3 -I $L/lab.py ssh $CELL main -- "sudo -n cat /root/tok.txt; sudo -n rm -f /root/tok.txt" > $T/tok.txt; chmod 600 $T/tok.txt; }
tokbody() { python3 -c 'import json,os,sys; t=open(sys.argv[1]).read().strip(); fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); d={"token":t}
if len(sys.argv)>3: d["new_password"]=json.load(open(sys.argv[3]))["password"]
os.write(fd,json.dumps(d).encode())' $T/tok.txt $T/tokbody.json "$@"; }
pub() { raw POST $1 --as none --body-file $T/tokbody.json --hdr Content-Type:application/json --show 400; }
req() { raw POST /api/v1/auth/mailbox-recovery/request --as none --json "{\"address\":\"$1\",\"lang\":\"en\"}" --hdr Content-Type:application/json --show 300; }
imapopen() { # an IMAP session of the mailbox from labnet for 120 s (password in a 0600 file, removed after login)
  pw $1 | timeout 30 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n sh -c 'umask 077; cat > /root/imap.pw'"
  timeout 30 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n systemd-run --unit lab-imap-$RANDOM python3 -c \"
import imaplib,ssl,time,os
p=open('/root/imap.pw').read().strip(); os.remove('/root/imap.pw')
c=ssl.create_default_context(); c.check_hostname=False; c.verify_mode=ssl.CERT_NONE
m=imaplib.IMAP4_SSL('$MIP',993,ssl_context=c); m.login('$2',p); m.select('INBOX')
for i in range(24):
    time.sleep(5)
    try: m.noop()
    except Exception as e: print('session ended', e); break
\"" > /dev/null 2>&1; sleep 4; }
for part in "$@"; do echo; echo "######## $part $(ts)"; case $part in
set)
  g count $RA
  as cust1 PUT /api/v1/domains/$C1/mail/accounts/recovery "{\"id\":$MB,\"address\":\"$RA\",\"lang\":\"tr\"}" | sbrief 600
  echo "-- self refused:"; as cust1 PUT /api/v1/domains/$C1/mail/accounts/recovery "{\"id\":$MB,\"address\":\"$MBA\",\"lang\":\"en\"}" | sbrief 300
  sleep 15; g count $RA; gettok
  tokbody; echo "-- confirm"; pub /api/v1/auth/mailbox-recovery/confirm
  as cust1 GET "/api/v1/domains/$C1/mail/accounts/recovery?id=$MB" | sbrief 600
  echo "-- the same confirmation link again:"; pub /api/v1/auth/mailbox-recovery/confirm ;;
request)
  g count $RA
  echo "-- known:"; req $MBA; echo "-- unknown:"; req nobody@cust1.$Z; echo "-- unknown domain:"; req who@nowhere.test
  sleep 15; g count $RA ;;
reset)
  g hashes > $T/hash-before.txt; cat $T/hash-before.txt
  gettok; tokbody; echo "-- link"; pub /api/v1/auth/mailbox-recovery/link
  imapopen cbox1 $MBA; g who $MBA
  mkcred cbox1new; tokbody $T/cred-cbox1new.json; echo "-- reset"; pub /api/v1/auth/mailbox-recovery/reset
  sleep 5; g who $MBA; g hashes > $T/hash-after.txt; echo "-- users lines changed:"; diff $T/hash-before.txt $T/hash-after.txt | grep -E '^[<>]'
  echo "-- the same link again (single use):"; pub /api/v1/auth/mailbox-recovery/reset
  echo "-- new password works over IMAP:"; pw cbox1new | timeout 30 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n python3 -c \"
import imaplib,ssl,sys
p=sys.stdin.readline().strip(); c=ssl.create_default_context(); c.check_hostname=False; c.verify_mode=ssl.CERT_NONE
m=imaplib.IMAP4_SSL('$MIP',993,ssl_context=c); print(m.login('$MBA',p)[0]); m.logout()\""
  cp $T/cred-cbox1new.json $T/cred-cbox1.json; g count $MBA ;;
oldlink)
  req $MBA; sleep 15; gettok; tokbody
  mkcred cbox1p2; python3 -c 'import json,os,sys; p=json.load(open(sys.argv[1]))["password"]; fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps({"id":int(sys.argv[3]),"new_password":p}).encode())' $T/cred-cbox1p2.json $T/pwb.json $MB
  echo "-- panel password change"; as cust1 PUT /api/v1/domains/$C1/mail/accounts/password @$T/pwb.json | sbrief 300; rm -f $T/pwb.json; cp $T/cred-cbox1p2.json $T/cred-cbox1.json
  echo "-- the older link:"; pub /api/v1/auth/mailbox-recovery/link ;;
suspended)
  g count $RA
  adm POST /api/v1/subscriptions/$SUB/suspension '{"action":"suspend","reason":"lab"}' --label susp --timeout 300 | sbrief 300
  req $MBA; sleep 20; g count $RA
  adm POST /api/v1/subscriptions/$SUB/suspension '{"action":"resume","reason":"lab"}' --label unsusp --timeout 300 | sbrief 300 ;;
oldreset)
  req $MBA; sleep 15; gettok; cp $T/tok.txt $T/tok-old.txt
  mkcred cbox1p3; python3 -c 'import json,os,sys; p=json.load(open(sys.argv[1]))["password"]; fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps({"id":int(sys.argv[3]),"new_password":p}).encode())' $T/cred-cbox1p3.json $T/pwb.json $MB
  echo "-- panel password change"; as cust1 PUT /api/v1/domains/$C1/mail/accounts/password @$T/pwb.json | sbrief 300; rm -f $T/pwb.json; cp $T/cred-cbox1p3.json $T/cred-cbox1.json
  g hashes > $T/hash-b.txt
  mkcred cbox1x; cp $T/tok-old.txt $T/tok.txt; tokbody $T/cred-cbox1x.json; echo "-- reset with the link made before the change:"; pub /api/v1/auth/mailbox-recovery/reset
  g hashes > $T/hash-c.txt; echo "-- users lines changed by that reset (expect none):"; diff $T/hash-b.txt $T/hash-c.txt | grep -E '^[<>]'; rm -f $T/tok-old.txt ;;
manual)
  ON='{"enabled":true,"mailbox_per_hour":500,"domain_per_hour":750,"subscription_per_hour":1000,"new_mailbox_per_hour":50,"new_mailbox_hours":24,"spike_factor":5,"spike_minimum":100,"refusal_pause":3}'
  echo "-- the administrator's manual stop of sales@ (lab: written as the screen's manual stop is stored)"
  grun main g_manual.sh
  sleep 75; adm GET /api/v1/mail/outgoing --label outgoing | sbrief 600
  echo "   send while stopped:"; pw cbox2 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP 587 sales@cust1.$Z box1@site1.$Z sales@cust1.$Z"
  rm -f $T/cred-cbox2.json; mkcred cbox2; python3 -c 'import json,os,sys; p=json.load(open(sys.argv[1]))["password"]; fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps({"id":3,"new_password":p}).encode())' $T/cred-cbox2.json $T/pwb.json
  echo "-- password change"; as cust1 PUT /api/v1/domains/$C1/mail/accounts/password @$T/pwb.json | sbrief 300; rm -f $T/pwb.json
  sleep 60; adm GET /api/v1/mail/outgoing --label outgoing | sbrief 600
  echo "   send after the change:"; pw cbox2 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP 587 sales@cust1.$Z box1@site1.$Z sales@cust1.$Z" ;;
manualsend)
  date -u +%T; gshn main 'grep -c "sales@cust1" /etc/postfix/postfwd.cf /var/lib/celikpanel-postfwd/* 2>/dev/null | head -3; journalctl -u celikpanel-panel --since 11:57 -o cat --no-pager | grep -i "mail-outgoing" | tail -4 | cut -c1-200'
  pw cbox2 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP 587 sales@cust1.$Z box1@site1.$Z sales@cust1.$Z" ;;
esac; done
rm -f $T/tok.txt $T/tokbody.json
echo "== k8 $* done $(ts)"
