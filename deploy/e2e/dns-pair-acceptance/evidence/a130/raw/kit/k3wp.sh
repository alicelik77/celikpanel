# alpha.129 case 3 (WordPress): a catalogue WordPress gets the new database password in its configuration. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C3wp-$CELL.log) 2>&1
echo "== 3w.0 site wp.$Z and WordPress from the catalogue $(ts)"
echo "   the administrator installs PHP 8.4 in Components (PHP-FPM 8.4), as APP_EXTENSIONS_MISSING said"; installc main php-fpm php8.4-fpm
W=$(domain_id wp.$Z main); [ -z "$W" ] && W=$(mksite main wp.$Z php); echo "wp id $W"; echo $W > $T/wp.id
admn main POST /api/v1/domains/$W/apps/install '{"app":"wordpress"}' --label wp-install --timeout 900 | sbrief 900
for i in $(seq 1 40); do s=$(admn main GET /api/v1/domains/$W/apps --label apps | sbrief 600); echo "$s" | grep -qiE '"(status|state)": "(applied|installed|ready|failed)' && break; sleep 15; done; echo "$s" | cut -c1-600
admn main GET /api/v1/domains/$W/databases --label wpdbs > $T/wpdbs.json; python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["body"][:900])' $T/wpdbs.json
read WDB WUSER WUID < <(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("databases") if isinstance(b,dict) else b; x=l[0]; u=(x.get("users") or [{}])[0]; print(x["id"], x.get("user") or u.get("name") or u.get("username"), u.get("id") or x.get("user_id") or "")' $T/wpdbs.json)
echo "wp database $WDB user $WUSER user-id $WUID"
gshn main "R=\$(ls -d /var/www/celikpanel/subscriptions/*/sites/$W/public_html); sha256sum \$R/wp-config.php | cut -c1-16; grep -c DB_PASSWORD \$R/wp-config.php"
echo "   site before: $(lab ssh beta1-lab labnet -- "curl -sS -m 20 -o /dev/null -w '%{http_code}' -H 'Host: wp.$Z' http://$MIP/" 2>/dev/null)"
echo "== 3w.1 new password for the WordPress database user $(ts)"
mkcred wpdb; python3 -c 'import json,os,sys; p=json.load(open(sys.argv[1]))["password"]; fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps({"password":p}).encode())' $T/cred-wpdb.json $T/pwbody.json
admn main POST /api/v1/domains/$W/databases/$WDB/users/$WUID/password @$T/pwbody.json --label wp-pass --timeout 300 | sbrief 900; rm -f $T/pwbody.json
pw wpdb | timeout 60 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash -c 'IFS= read -r P; R=\$(ls -d /var/www/celikpanel/subscriptions/*/sites/$W/public_html); grep -F \"DB_PASSWORD\" \$R/wp-config.php | grep -cF \"\$P\" | sed \"s/^/wp-config holds the new password: /\"; sha256sum \$R/wp-config.php | cut -c1-16'"
echo "   site after: $(lab ssh beta1-lab labnet -- "curl -sS -m 20 -o /dev/null -w '%{http_code}' -H 'Host: wp.$Z' http://$MIP/" 2>/dev/null)"
echo "== k3wp done $(ts)"
