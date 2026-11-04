CELL=a118c IP=192.0.3.192 Z=c118arch.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/C1-$CELL.log) 2>&1
echo "== C (Arch) checks $(ts)"
mkcred custc
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-custc.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-custc | sbrief 200; rm -f $T/u.json
SUB=$(sub_of custc); DID=$(newdomain custc $SUB ca118.test); echo "ca118.test id=$DID"
mkcred mbx-q
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"address":"q@ca118.test","password":c["password"],"quota_mb":1},open(sys.argv[2],"w"))' $T/cred-mbx-q.json $T/m.json
as custc POST /api/v1/domains/$DID/mail/accounts @$T/m.json | sbrief 200; rm -f $T/m.json
lc fetch - ca118.test ""
adm GET /api/v1/usage/status --label usage-status | sbrief 800
cat > $A/gC.sh <<'G'
echo "## $(date -u +%T) $(grep PRETTY /etc/os-release)"; nginx -t 2>&1 | tail -1
cat /etc/nginx/celikpanel-managed.d/ca118.test/access-log.traffic 2>&1 | grep -v '^#'; ls -la /var/log/celikpanel/traffic/ 2>&1 | grep -v total
echo "## dovecot $(dovecot --version)"; doveconf -n >/dev/null 2>/tmp/dc.err; echo "doveconf rc=$? $(head -2 /tmp/dc.err)"
ls /etc/dovecot/conf.d/ 2>&1 | tr '\n' ' '; echo; cat /etc/dovecot/conf.d/90-celikpanel-quota.conf 2>&1 | grep -v '^#' | head -30
postconf -h smtpd_recipient_restrictions; command -v logrotate || echo "no logrotate"
G
GQT=60 gq gC.sh
