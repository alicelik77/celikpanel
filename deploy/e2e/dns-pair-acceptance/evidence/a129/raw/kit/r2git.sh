# alpha.128 round 2: (A4) Git deploy without git gives the panel's own refusal; Git installed from Components; deploy
# over SSH with the deploy key works; (A6) a domain made and deleted with an API token finishes by itself. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R2git-$CELL.log) 2>&1
N=main; D=gitx.$Z
raw() { python3 -I $A/raw.py $L $CELL main "$@"; }
gitdeploy() { local R="git-$(rid)"; admn $N POST /api/v1/domains/$DID/git/deploy "{\"request_id\":\"$R\"}" --label git-deploy --timeout 300 | sbrief 400
  for i in $(seq 1 50); do s=$(admn $N GET /api/v1/domains/$DID/git --label git-st | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); s=b.get("status") or {}; print(s.get("phase"), s.get("reason") or "", (s.get("error") or "")[:160], "|", " / ".join((s.get("lines") or [])[-3:])[:300])'); case "$s" in running*) sleep 6;; *) break;; esac; done; echo "   deploy: $s"; }
echo "== G.0 lab Git server $(ts)"
P=$B/portalnet; ( cd $P && openssl req -new -newkey rsa:2048 -nodes -subj /CN=git.lab.test -keyout git-tls-key.pem -out git.csr 2>/dev/null \
  && printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectAltName=DNS:git.lab.test\n' > git.ext \
  && openssl x509 -req -in git.csr -CA ca.pem -CAkey ca-key.pem -set_serial 9 -days 5 -extfile git.ext -out git-tls.pem 2>/dev/null && chmod 600 git-tls-key.pem )
lab ssh beta1-lab labnet -- "sudo -n tee /root/git-tls.pem >/dev/null" < $P/git-tls.pem
lab ssh beta1-lab labnet -- "sudo -n sh -c 'umask 077; cat > /root/git-tls-key.pem'" < $P/git-tls-key.pem
lab ssh beta1-lab labnet -- "sudo -n tee /root/git-smart.py >/dev/null" < $A/n_gitsmart.py
nup n_git.sh; NRT=120 nrun n_git.sh setup >/dev/null
lab ssh beta1-lab labnet -- "sudo -n sh -c 'systemctl stop a128-git; systemd-run --unit a128-git -p RuntimeMaxSec=28800 python3 -I /root/git-smart.py >/dev/null 2>&1; sleep 1; systemctl is-active a128-git'"
gshn $N 'grep -q git.lab.test /etc/hosts || echo "192.0.2.62 git.lab.test # a128 lab git" >> /etc/hosts; command -v git || echo "no git on the server"'
DID=$(mksite $N $D static); echo "$D id $DID"
admn $N POST /api/v1/domains/$DID/git '{"repo_url":"https://git.lab.test/app.git","branch":"main","build":false}' --label git-set | sbrief 200
echo "== G.1 (A4) deploy without git $(ts)"; gitdeploy
admn $N GET /api/v1/managed-services --label ms | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); rows=b if isinstance(b,list) else (b.get("services") or []); [print("   component", json.dumps({k:r.get(k) for k in ("id","name","kind","category","is_installed","status")})) for r in rows if r.get("id")=="git"]'
echo "== G.2 install Git from Components $(ts)"; installc $N git
gshn $N 'git --version'
echo "== G.3 HTTPS deploy $(ts)"; gitdeploy
grun $N g_curl.sh $D /server.js | head -2
echo "== G.4 (A6) a domain made and deleted with an API token on the pair's main $(ts)"
CK=$T/cred-cust1.json; as cust1 GET /api/v1/account/api-tokens > /dev/null
adm PUT /api/v1/panel/remote-api '{"enabled":true}' --label rapi-on | sbrief 100
python3 -c 'import json,os,sys; c=json.load(open(sys.argv[1])); fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps({"name":"lab","scope":"manage","expires_days":7,"password":c["password"]}).encode())' $CK $T/tokreq.json
raw POST /api/v1/account/api-tokens --as $CK --json-file $T/tokreq.json --hdr X-CelikPanel-Request-Id:$(rid) --out $T/tokresp.json
python3 -c 'import json,os,sys; d=json.load(open(sys.argv[1])); fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,d.get("token","").encode()); print("   token made:", bool(d.get("token")))' $T/tokresp.json $T/tok-mg.txt; rm -f $T/tokresp.json $T/tokreq.json
infra delegate tokdel.$Z ns1.$Z $MIP ns2.$Z $BIP > /dev/null
SUB=$(cat $T/sub-cust1.id)
RAW_TIMEOUT=300 raw POST /api/v1/domains/create --as none --bearer-file $T/tok-mg.txt --json "{\"domain\":\"tokdel.$Z\",\"project_type\":\"static\",\"subscription_id\":$SUB}" --show 250
sleep 20; TD=$(domain_id tokdel.$Z main); echo "   tokdel id $TD"
RAW_TIMEOUT=600 raw DELETE "/api/v1/domains/$TD?confirm_domain=tokdel.$Z" --as none --bearer-file $T/tok-mg.txt --show 250
for i in $(seq 1 24); do n=$(raw GET /api/v1/domains --as none --bearer-file $T/tok-mg.txt --show 4000 | grep -c "tokdel"); [ "$n" = 0 ] && { echo "   tokdel gone from the token's list after $((i*10)) s"; break; }; sleep 10; done
[ "$n" != 0 ] && { echo "   tokdel still listed"; admn main GET /api/v1/domains/$TD/deletion-status --label delst | sbrief 400; }
gshn main 'journalctl -u celikpanel-panel --since -6min -o cat --no-pager | grep -iE "tokdel" | tail -4 | cut -c1-240'
echo "== r2git done $(ts)"
