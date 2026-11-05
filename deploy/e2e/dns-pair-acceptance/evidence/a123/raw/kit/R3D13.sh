# alpha.123 case 3 on Debian 13 (d13): the component dependency rule, remove-together with re-attach after a
# reload and after a panel restart, PHP version, Certbot and nftables refusals, Node 24 with n8n, Fail2ban without
# the firewall engine. Lab only.
export CELL=d13 IP=192.0.3.192 Z=d13.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/lib.sh
source $A/lib11.sh; source $A/lib123.sh
exec > >(tee -a $LOGS/R3-$CELL.log) 2>&1
IDS=postgresql,phppgadmin,phpmyadmin,php-fpm,fail2ban,nftables,certbot,node,nginx,mariadb
echo "== 3.0 components $(ts)"; comps $IDS
echo "== 3.1 install PostgreSQL, phpPgAdmin, phpMyAdmin, Fail2ban $(ts)"
for s in postgresql phppgadmin phpmyadmin fail2ban; do installc $s; done
comps $IDS
echo "== 3.2 remove PostgreSQL: the plan, then without confirming the together list $(ts)"
impact "service_id=postgresql"; TOG=$(together_of); echo "  together=$TOG"
uninst '{"service_id":"postgresql"}'
echo "== 3.3 remove together; the tab is closed at once and a new page reads the operation $(ts)"
R=$(rid); adm POST /api/v1/service/uninstall "{\"service_id\":\"postgresql\",\"remove_together\":$TOG,\"request_id\":\"$R\"}" --label rt1 > $T/rt1.json; sbrief 700 < $T/rt1.json
OID=$(jget operation.id < $T/rt1.json)
echo "-- new page (active read)"; opread "active=1"
echo "-- reload (the tab's marker, by request id)"; opread "request_id=$R"
follow $OID; comps $IDS
echo "== 3.4 again with a panel restart in the middle $(ts)"
for s in postgresql phppgadmin; do installc $s; done; comps postgresql,phppgadmin
impact "service_id=postgresql"; TOG=$(together_of)
R=$(rid); adm POST /api/v1/service/uninstall "{\"service_id\":\"postgresql\",\"remove_together\":$TOG,\"request_id\":\"$R\"}" --label rt2 > $T/rt2.json; sbrief 700 < $T/rt2.json
OID=$(jget operation.id < $T/rt2.json); opread "id=$OID"
echo "-- panel restart (as the owner would: systemctl restart celikpanel-panel) $(ts)"
gsh1 'systemctl restart celikpanel-panel; systemctl is-active celikpanel-panel'
panelup
echo "-- new page after the restart (active read)"; opread "active=1"
echo "-- tab marker after the restart"; opread "request_id=$R"
follow $OID; comps $IDS
gsh1 'journalctl -u celikpanel-panel --since "-5min" -o cat --no-pager | grep -iE "remove.together|resum|service_remove" | tail -8'
echo "== 3.5 a site on PHP, then remove the PHP version phpMyAdmin and the site use $(ts)"
mkcred cust123
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-cust123.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-cust123 | sbrief 200; rm -f $T/u.json
SUB=$(sub_of cust123); echo $SUB > $T/sub
infra delegate site123.test ns1.$Z $IP ns2.$Z $IP > /dev/null
DID=$(newdomain cust123 $SUB site123.test); echo "site123.test id=$DID"; echo $DID > $T/did
PV=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); L=b if isinstance(b,list) else (b.get("services") or []); print([ (s.get("versions") or [""])[0] for s in L if s.get("id")=="php-fpm"][0])' $T/ms.json); echo "php version $PV"
impact "service_id=php-fpm&package=php$PV-fpm"
uninst "{\"service_id\":\"php-fpm\",\"package\":\"php$PV-fpm\"}"
echo "== 3.6 Certbot and nftables $(ts)"
impact "service_id=certbot"; uninst '{"service_id":"certbot"}'
impact "service_id=nftables"; uninst '{"service_id":"nftables"}'
comps $IDS
echo "== R3 part 1 done $(ts)"
