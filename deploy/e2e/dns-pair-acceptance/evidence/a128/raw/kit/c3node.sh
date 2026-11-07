# alpha.128 case 3b: Node.js environment variables, a package script, Git deployment over HTTPS (lab Git server with a
# lab-CA certificate) and over SSH (the generated deploy key) on the updated main. Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C3node-$CELL.log) 2>&1
N=main; D=node8.$Z
site() { lab ssh beta1-lab labnet -- "curl -sS -m 10 -H 'Host: $D' http://$MIP/" 2>/dev/null; }
echo "== 3b.0 lab Git server $(ts)"
P=$B/portalnet; ( cd $P && openssl req -new -newkey rsa:2048 -nodes -subj /CN=git.lab.test -keyout git-tls-key.pem -out git.csr 2>/dev/null \
  && printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectAltName=DNS:git.lab.test\n' > git.ext \
  && openssl x509 -req -in git.csr -CA ca.pem -CAkey ca-key.pem -set_serial 7 -days 5 -extfile git.ext -out git-tls.pem 2>/dev/null && chmod 600 git-tls-key.pem )
lab ssh beta1-lab labnet -- "sudo -n tee /root/git-tls.pem >/dev/null" < $P/git-tls.pem
lab ssh beta1-lab labnet -- "sudo -n sh -c 'umask 077; cat > /root/git-tls-key.pem'" < $P/git-tls-key.pem
nup n_git.sh; NRT=120 nrun n_git.sh setup
gshn $N 'grep -q git.lab.test /etc/hosts || echo "192.0.2.62 git.lab.test # a128 lab git" >> /etc/hosts; curl -sS -m 10 https://git.lab.test/app.git/info/refs | head -2'
echo "== 3b.1 Node.js runtime and a Node site $(ts)"
admn $N GET /api/v1/runtimes/node/lts --label lts > $T/lts.json
V=$(admn $N GET /api/v1/runtimes/node --label nodes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print((b.get("installed") or [""])[0])')
if [ -z "$V" ]; then
  V=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); r=b["releases"]; print(([x["version"] for x in r if x["version"].startswith("22.")] or [r[0]["version"]])[0])' $T/lts.json)
  for k in $(seq 1 30); do r=$(admn $N POST /api/v1/runtimes/node "{\"version\":\"$V\",\"request_id\":\"$(rid)\"}" --label node-install --timeout 900 | sbrief 300); echo "$r"; echo "$r" | grep -q service_operation_busy || break; sleep 20; done
  for i in $(seq 1 60); do admn $N GET /api/v1/runtimes/node --label nodes | grep -q "$V" && break; sleep 10; done
fi
echo "node $V"
DID=$(mksite $N $D static); echo "$D id $DID"; echo $DID > $T/node8.id
admn $N GET /api/v1/domains/$DID/git --label git-get | sbrief 600
echo "== 3b.2 Git over HTTPS into the static site, then Node $(ts)"
admn $N POST /api/v1/domains/$DID/git '{"repo_url":"https://git.lab.test/app.git","branch":"main","build":false}' --label git-set | sbrief 400
gitdeploy() { local R="git-$(rid)"; admn $N POST /api/v1/domains/$DID/git/deploy "{\"request_id\":\"$R\"}" --label git-deploy --timeout 300 | sbrief 500
  for i in $(seq 1 40); do s=$(admn $N GET /api/v1/domains/$DID/git --label git-st | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); s=b.get("status") or {}; print(s.get("phase"), s.get("reason") or "", s.get("class") or "", (s.get("error") or "")[:200], "|", " / ".join((s.get("lines") or [])[-4:])[:600])'); case "$s" in running*) sleep 6;; *) break;; esac; done; echo "   deploy: $s"; }
gitdeploy
admn $N PUT /api/v1/domains/$DID/hosting "{\"project_type\":\"node\",\"start_command\":\"node server.js\",\"runtime_version\":\"$V\"}" --label to-node --timeout 600 | sbrief 400
sleep 8; echo "   site: $(site)"
echo "== 3b.3 environment variables (one secret) $(ts)"
admn $N POST /api/v1/domains/$DID/app/env '{"entries":[{"key":"NODE_ENV","value":"production","secret":false},{"key":"LAB_PUBLIC","value":"a128-public","secret":false},{"key":"LAB_SECRET","value":"s3cret-value-a128","secret":true}]}' --label env-set --timeout 300 | sbrief 400
admn $N GET /api/v1/domains/$DID/app/env --label env-get | sbrief 500
sleep 5; echo "   site: $(site)"
grun $N g_nenv.sh $DID
echo "== 3b.4 a package script (build) $(ts)"
admn $N GET /api/v1/domains/$DID/app/packages --label pk | sbrief 400
admn $N POST /api/v1/domains/$DID/app/packages "{\"request_id\":\"node-$(rid)\",\"build\":false,\"script\":\"build\"}" --label pk-run --timeout 600 | sbrief 400
for i in $(seq 1 30); do s=$(admn $N GET /api/v1/domains/$DID/app/packages --label pk-st | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(b.get("phase"), b.get("script"), b.get("reason") or "", "|", " / ".join((b.get("lines") or [])[-3:])[:300])'); case "$s" in running*) sleep 6;; *) break;; esac; done; echo "   script: $s"
admn $N POST /api/v1/domains/$DID/app/restart '{}' --label restart | sbrief 200; sleep 5; echo "   site: $(site)"
echo "== 3b.5 Git over SSH with the generated deploy key $(ts)"
admn $N POST /api/v1/domains/$DID/git/key '{}' --label git-key > $T/gitkey.json; python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); print("  ", d["status"], b.get("fingerprint"), (b.get("public_key") or "")[:30]); open(sys.argv[2],"w").write((b.get("public_key") or "")+"\n")' $T/gitkey.json $T/deploy.pub
echo "   key again (no replace):"; admn $N POST /api/v1/domains/$DID/git/key '{}' --label git-key2 | sbrief 300
lab ssh beta1-lab labnet -- "sudo -n bash /root/n_git.sh addkey" < $T/deploy.pub
NRT=60 nrun n_git.sh commit ssh-2
admn $N POST /api/v1/domains/$DID/git '{"repo_url":"git@git.lab.test:app.git","branch":"main","build":true}' --label git-set-ssh | sbrief 400
gitdeploy
admn $N POST /api/v1/domains/$DID/app/restart '{}' --label restart | sbrief 200; sleep 5; echo "   site: $(site)"
NRT=60 nrun n_git.sh log 8
gshn $N 'systemctl list-units --failed --no-legend --plain | cat; echo "(failed end)"'
echo "== c3node done $(ts)"
