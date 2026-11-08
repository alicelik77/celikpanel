# alpha.128 case 2h: plan Basic "notify only": cust1 goes over its disk limit; after the usage round the site still
# answers and the account is not suspended; a notice is left. Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C2notify-$CELL.log) 2>&1
C1=$(cat $T/cust1.id); PID=$(cat $T/planBasic.id); SUB=$(cat $T/sub-cust1.id)
gshn main "R=\$(ls -d /var/www/celikpanel/subscriptions/*/sites/$C1/public_html | head -1); head -c 40M /dev/urandom > \$R/lab-big.bin; chown --reference=\$R \$R/lab-big.bin; du -sh \$R"
adm PUT /api/v1/plans/$PID '{"name":"Basic","max_domains":3,"max_databases":3,"max_email_accounts":10,"disk_quota_mb":20,"bandwidth_quota_mb":0,"overuse_policy":"notify","notify_percent_disk":60,"accept_over_usage":true,"confirm_narrowing":true}' --label plan-small | sbrief 500
waitround; waitround
subusage cust1
adm GET /api/v1/subscriptions --label subs | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  ", json.dumps(s)[:500]) for s in b["subscriptions"] if s.get("owner")=="cust1"]'
echo "   site: $(lab ssh beta1-lab labnet -- "curl -sS -m 10 -o /dev/null -w '%{http_code}' -H 'Host: cust1.$Z' http://$MIP/" 2>/dev/null)"
adm GET /api/v1/rights/notices --label notices | sbrief 1500
echo "   upload while over the disk limit (file manager write):"
as cust1 GET "/api/v1/domains/$C1/usage" | sbrief 400
adm PUT /api/v1/plans/$PID '{"name":"Basic","max_domains":3,"max_databases":3,"max_email_accounts":10,"disk_quota_mb":2048,"bandwidth_quota_mb":0,"accept_over_usage":true,"confirm_narrowing":true}' --label plan-restore | sbrief 300
gshn main "rm -f /var/www/celikpanel/subscriptions/*/sites/$C1/public_html/lab-big.bin"
echo "== c2notify done $(ts)"
