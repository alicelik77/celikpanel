# alpha.125 round, before the update (pair at 124): case 3 baseline, sites, mailboxes and crontabs that exist before the update. Lab only.
export CELL=p5 IP=192.0.3.190 Z=p5.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/pre-$CELL.log) 2>&1
P=$B/perf; mkdir -p $P
echo "== versions $(ts)"; ver main; ver backup
echo "== case 3 baseline on alpha.124 (main Debian 13, backup Ubuntu 24.04) $(ts)"
for n in main backup; do python3 -I $A/perf.py $L $CELL $n a124-$n $P; done
bash $A/gz.sh $CELL main
echo "== a customer PHP site that exists before the update: shop5.test $(ts)"
mkcred cust5; mkcred mbx5; mkcred mbx5in
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-cust5.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-cust5 | sbrief 200; rm -f $T/u.json
SUB=$(sub_of cust5); echo "subscription $SUB"
infra delegate shop5.test ns1.$Z $IP ns2.$Z 192.0.3.191 > /dev/null
SID=$(newdomain cust5 $SUB shop5.test); echo "shop5.test id=$SID"; echo $SID > $T/sid
MID=$(domain_id $Z); echo "$Z id=$MID"; echo $MID > $T/mid
sleep 15
as cust5 GET /api/v1/domains/$SID/php | sbrief 300
echo "== mailboxes: inbox5@$Z (receives) and noreply@shop5.test (sends on 587) $(ts)"
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"address":"inbox5@'$Z'","password":c["password"],"quota_mb":50},open(sys.argv[2],"w"))' $T/cred-mbx5in.json $T/m.json
adm POST /api/v1/domains/$MID/mail/accounts @$T/m.json --label create-inbox | sbrief 200
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"address":"noreply@shop5.test","password":c["password"],"quota_mb":50},open(sys.argv[2],"w"))' $T/cred-mbx5.json $T/m.json
adm POST /api/v1/domains/$SID/mail/accounts @$T/m.json --label create-sender | sbrief 200; rm -f $T/m.json
echo "== crontabs before the update: shop5.test written by the panel; $Z written by the panel, then the owner adds MAILTO by hand $(ts)"
read SDR SU < <(siteof shop5.test); read MDR MU < <(siteof $Z); echo "shop5.test $SDR $SU | $Z $MDR $MU"; echo "$SU $MU" > $T/users
cronadd $SID "*/30 * * * *" "/usr/bin/php -v > /dev/null"
cronadd $MID "15 3 * * *" "/bin/true"
grun main g_cron.sh owner $MU webmaster@$Z
grun main g_cron.sh show $SU $MU
gshn main 'postconf authorized_submit_users smtpd_relay_restrictions mynetworks; ls -la /var/lib/celikpanel-agent-private/mail-passwordless.json 2>&1 | tail -1'
echo "== pre done $(ts)"
