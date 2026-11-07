# alpha.128 round 2, B items on the updated pair: notify-only over-limit, expiry warning, notifications (all sources,
# seen), sessions + idle timeout + administrator address list (recovery file, API token from an unlisted address),
# activity log (customer/reseller), domain health strip, maintenance (clock, processes), certificate request -> install
# with a lab-CA certificate. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R2b-$CELL.log) 2>&1
raw() { python3 -I $A/raw.py $L $CELL main "$@"; }
S1=$(cat $T/site1.id); C1=$(cat $T/cust1.id); PID=$(cat $T/planBasic.id)
notif() { python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]) if d["body"].startswith("{") else {}
print("  ", d["status"], "items", len(b.get("items") or []), "monitoring", b.get("monitoring"))
for i in b.get("items") or []: print("    ", json.dumps({k:i.get(k) for k in ("source","id","kind","audience","needs_action","fix_path","usage","clamp")})[:330])'; }
echo "== B.1 notify-only: a new customer cust2 on plan Notify (disk 20 MB, notify), its site over the limit $(ts)"
adm POST /api/v1/plans '{"name":"Notify","max_domains":2,"max_databases":1,"max_email_accounts":2,"disk_quota_mb":20,"bandwidth_quota_mb":0,"overuse_policy":"notify","notify_percent_disk":60}' --label plan-notify | sbrief 200
NP=$(adm GET /api/v1/plans --label plans | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("plans") if isinstance(b,dict) else b; print([p["id"] for p in l if p.get("name")=="Notify"][0])')
mkcred cust2; adm POST /api/v1/users "{\"username\":\"cust2\",\"email\":\"cust2@lab.test\",\"password\":\"$(pw cust2)\",\"role\":\"customer\",\"plan_id\":$NP}" --label u2 | sbrief 200
SUB2=$(sub_of cust2); echo "cust2 subscription $SUB2"
infra delegate cust2.$Z ns1.$Z $MIP ns2.$Z $BIP > /dev/null
as cust2 POST /api/v1/domains/create "{\"domain\":\"cust2.$Z\",\"project_type\":\"php\",\"ssl_type\":\"none\",\"subscription_id\":$SUB2}" | sbrief 200
C2=$(domain_id cust2.$Z main); echo "cust2 site $C2"
gshn main "R=\$(ls -d /var/www/celikpanel/subscriptions/*/sites/$C2/public_html | head -1); head -c 30M /dev/urandom > \$R/lab-big.bin; chown --reference=\$R \$R/lab-big.bin; du -sh \$R"
echo "-- expiry in 5 days (also kicks the usage worker) $(ts)"
END=$(date -u -d '+5 days' +%F); adm PUT /api/v1/subscriptions/$SUB2/ends-at "{\"ends_at\":\"$END\"}" --label ends-at | sbrief 300
for i in $(seq 1 30); do sleep 10; u=$(as cust2 GET /api/v1/notifications | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(" ".join(sorted(i["kind"] for i in b.get("items") or [])))'); case "$u" in *disk_100*) echo "  notices after $((i*10)) s: $u"; break;; esac; done; echo "  cust2 notices: $u"
as cust2 GET /api/v1/notifications | notif
echo "   cust2 site: $(lab ssh beta1-lab labnet -- "curl -sS -m 10 -o /dev/null -w '%{http_code}' -H 'Host: cust2.$Z' http://$MIP/" 2>/dev/null) (notify only: still open)"
adm GET /api/v1/subscriptions --label subs | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  ", json.dumps(s)[:420]) for s in b["subscriptions"] if s.get("owner")=="cust2"]'
echo "-- seen round trip (expiry notice via /api/v1/notifications/{id}/seen, usage via /api/v1/usage-notices/{id}/seen)"
read EID UID2 < <(as cust2 GET /api/v1/notifications | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); it=b.get("items") or []; e=[i["id"] for i in it if i["source"]=="account"]; u=[i["id"] for i in it if i["source"]=="usage"]; print(e[0] if e else 0, u[0] if u else 0)')
as cust2 POST /api/v1/notifications/$EID/seen '{}' | sbrief 100; as cust2 POST /api/v1/usage-notices/$UID2/seen '{}' | sbrief 100
as cust2 GET /api/v1/notifications | notif
echo "== B.2 administrator's notifications (usage, site_limit clamp, mail_outgoing) $(ts)"; adm GET /api/v1/notifications --label notif | notif
echo "== B.3 domain health strip $(ts)"
for d in $S1 $C1 $C2; do adm GET /api/v1/domains/$d/health --label health | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("  ", d["status"], b.get("domain"), b.get("contact"), [(c.get("id"), c.get("state"), c.get("code")) for c in b.get("checks") or []])'; done
echo "== B.4 activity log: customer and reseller $(ts)"
as cust1 GET "/api/v1/me/activity?limit=5" | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("  cust1", d["status"], [(e.get("action"), (e.get("actor") or {}).get("kind")) for e in b.get("entries") or []])'
mkcred res1; adm POST /api/v1/users "{\"username\":\"res1\",\"email\":\"res1@lab.test\",\"password\":\"$(pw res1)\",\"role\":\"reseller\"}" --label res1 | sbrief 200
mkcred rc1; as res1 POST /api/v1/users "{\"username\":\"rc1\",\"email\":\"rc1@lab.test\",\"password\":\"$(pw rc1)\",\"role\":\"customer\"}" | sbrief 200
as rc1 GET /api/v1/subscriptions > /dev/null
as res1 GET "/api/v1/me/activity?limit=8" | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("  res1", d["status"], [(e.get("action"), (e.get("actor") or {}).get("kind"), (e.get("actor") or {}).get("name")) for e in b.get("entries") or []])'
as cust1 GET "/api/v1/audit-logs" | sbrief 120
echo "== B.5 sessions, idle timeout, administrator address list $(ts)"
as cust1 GET /api/v1/auth/sessions | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("  cust1 sessions", d["status"], len(b.get("sessions") or []), "idle", b.get("idle_minutes"))'
adm GET /api/v1/panel/sessions --label psess > $T/psess.json; python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); s=b.get("sessions") or []; print("  admin sees", len(s), "sessions of", sorted(set(x.get("username") for x in s)))' $T/psess.json
H=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print([x["handle"] for x in b["sessions"] if x.get("username")=="cust1"][0])' $T/psess.json)
adm POST /api/v1/panel/sessions/end "{\"handle\":\"$H\"}" --label psess-end | sbrief 120
echo "   cust1 with the ended session:"; python3 -I $A/raw.py $L $CELL main GET /api/v1/auth/sessions --as $T/cred-cust1.json --show 160
adm GET /api/v1/panel/access --label access | sbrief 600
ME=$(adm GET /api/v1/panel/access --label access | python3 -c 'import json,sys; print(json.loads(json.load(sys.stdin)["body"])["your_address"])'); echo "   my address $ME"
adm PUT /api/v1/panel/access '{"idle_minutes":15,"allowlist":["198.51.100.7"]}' --label access-lock | sbrief 300
adm PUT /api/v1/panel/access "{\"idle_minutes\":15,\"allowlist\":[\"$ME/32\"]}" --label access-set | sbrief 300
echo "-- an administrator token from the lab network (unlisted)"; adm PUT /api/v1/panel/remote-api '{"enabled":true}' --label rapi | sbrief 80
python3 -c 'import json,os,sys; st=json.load(open(sys.argv[1])); fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps({"name":"adm","scope":"read","expires_days":7,"password":st["password"]}).encode())' /var/tmp/cp-beta1/state/$CELL/main.json $T/tokreq.json
raw POST /api/v1/account/api-tokens --as admin --json-file $T/tokreq.json --hdr X-CelikPanel-Request-Id:$(rid) --out $T/tokresp.json; rm -f $T/tokreq.json
python3 -c 'import json,os,sys; d=json.load(open(sys.argv[1])); fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,d.get("token","").encode()); print("   admin token made:", bool(d.get("token")), d.get("code",""))' $T/tokresp.json $T/tok-adm.txt; rm -f $T/tokresp.json
PORT=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("panel_port"))' /var/tmp/cp-beta1/state/$CELL/main.json)
lab ssh beta1-lab labnet -- "sudo -n sh -c 'umask 077; cat > /root/tok-adm.txt'" < $T/tok-adm.txt
fromlab() { lab ssh beta1-lab labnet -- "curl -sk -m 10 -H \"Authorization: Bearer \$(sudo -n cat /root/tok-adm.txt)\" https://$MIP:$PORT/api/v1/domains | head -c 200; echo" 2>/dev/null; }
echo "   token from the lab network: $(fromlab)"
echo "   token from the allowed address: $(raw GET /api/v1/domains --as none --bearer-file $T/tok-adm.txt --show 80)"
echo "-- recovery file on the server (the owner's terminal)"
gshn main 'touch /etc/celikpanel/panel-access-open'; sleep 7
echo "   token from the lab network with the file: $(fromlab)"
adm GET /api/v1/panel/access --label access | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("   recovery_open", b.get("recovery_open"))'
gshn main 'rm -f /etc/celikpanel/panel-access-open'; sleep 7
echo "   token from the lab network without the file: $(fromlab)"
lab ssh beta1-lab labnet -- "sudo -n rm -f /root/tok-adm.txt"
adm PUT /api/v1/panel/access '{"idle_minutes":15,"allowlist":[]}' --label access-open | sbrief 200
echo "-- an idle session: cust2 signs in now and is asked again after the idle time (checked at the end of the round)"
as cust2 POST /api/v1/auth/activity '{}' | sbrief 60; date +%s > $T/cust2-idle-start
echo "== B.6 maintenance on the main $(ts)"
adm GET /api/v1/maintenance/clock --label clock | sbrief 400
adm GET /api/v1/maintenance/processes --label procs | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("  ", d["status"], b.get("state"), "vcpus", b.get("vcpus"), "top cpu", [(p.get("name"), p.get("cpu")) for p in (b.get("by_cpu") or [])[:4]])'
echo "== B.7 certificate request -> lab-CA certificate -> installed on csr8.$Z $(ts)"
CS=$(mksite main csr8.$Z php); echo "csr8 id $CS"
adm POST /api/v1/maintenance/certificate-requests "{\"names\":[\"csr8.$Z\",\"www.csr8.$Z\"]}" --label csr > $T/csr.json; python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); open(sys.argv[2],"w").write(b.get("csr_pem","")); print("  ", d["status"], b.get("id"), b.get("common_name"), b.get("key_type"), b.get("key_file"))' $T/csr.json $T/csr8.csr
RQ=$(python3 -c 'import json,sys; print(json.loads(json.load(open(sys.argv[1]))["body"]).get("id"))' $T/csr.json)
P=$B/portalnet; printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectAltName=DNS:csr8.%s,DNS:www.csr8.%s\n' $Z $Z > $T/csr8.ext
openssl x509 -req -in $T/csr8.csr -CA $P/ca.pem -CAkey $P/ca-key.pem -set_serial 88 -days 5 -extfile $T/csr8.ext -out $T/csr8.pem 2>&1 | tail -1
python3 -c 'import json,sys; json.dump({"target":"site","domain_id":int(sys.argv[3]),"certificate":open(sys.argv[1]).read(),"chain":open(sys.argv[2]).read()}, open(sys.argv[4],"w"))' $T/csr8.pem $P/ca.pem $CS $T/csrinst.json
raw POST /api/v1/maintenance/certificate-requests/$RQ/install --json-file $T/csrinst.json --hdr X-CelikPanel-Request-Id:$(rid) --show 300
echo "   install again:"; raw POST /api/v1/maintenance/certificate-requests/$RQ/install --json-file $T/csrinst.json --show 200
gshn main "echo | openssl s_client -connect 127.0.0.1:443 -servername csr8.$Z 2>/dev/null | openssl x509 -noout -subject -issuer -serial"
adm GET /api/v1/maintenance/certificates --label certs | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  ", json.dumps({k:c.get(k) for k in ("kind","name","source","issuer","state","days_left")})) for c in b.get("certificates") or [] if "csr8" in json.dumps(c)]; print("   requests", [(r.get("id"), r.get("state"), r.get("installed_for")) for r in b.get("requests") or []])'
echo "== r2b done $(ts)"
