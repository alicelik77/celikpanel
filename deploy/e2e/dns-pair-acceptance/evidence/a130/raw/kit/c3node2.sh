# alpha.128 case 3b (again): git installed by the owner from the terminal (no Components entry offers it), then Git
# deployment over HTTPS and over SSH with the deploy key, environment variables and a package script. Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C3node-$CELL.log) 2>&1
N=main; D=node8.$Z; DID=$(cat $T/node8.id)
site() { lab ssh beta1-lab labnet -- "curl -sS -m 10 -H 'Host: $D' http://$MIP/" 2>/dev/null | head -c 400; }
gitdeploy() { local R="git-$(rid)"; admn $N POST /api/v1/domains/$DID/git/deploy "{\"request_id\":\"$R\"}" --label git-deploy --timeout 300 | sbrief 400
  for i in $(seq 1 50); do s=$(admn $N GET /api/v1/domains/$DID/git --label git-st | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); s=b.get("status") or {}; print(s.get("phase"), s.get("reason") or "", s.get("class") or "", (s.get("error") or "")[:200], "|", " / ".join((s.get("lines") or [])[-5:])[:700])'); case "$s" in running*) sleep 6;; *) break;; esac; done; echo "   deploy: $s"; }
echo "== 3c.0 the owner installs git from the terminal (lab load: apt-get install git) $(ts)"
gshn $N 'DEBIAN_FRONTEND=noninteractive apt-get install -y -q git >/dev/null 2>&1; git --version'
echo "== 3c.1 HTTPS deploy into the Node site $(ts)"
admn $N POST /api/v1/domains/$DID/git '{"repo_url":"https://git.lab.test/app.git","branch":"main","build":false}' --label git-set | sbrief 300
gitdeploy
admn $N POST /api/v1/domains/$DID/app/restart '{}' --label restart | sbrief 200; sleep 6; echo "   site: $(site)"
echo "== 3c.2 package script build $(ts)"
admn $N GET /api/v1/domains/$DID/app/packages --label pk | sbrief 400
admn $N POST /api/v1/domains/$DID/app/packages "{\"request_id\":\"node-$(rid)\",\"build\":false,\"script\":\"build\"}" --label pk-run --timeout 600 | sbrief 300
for i in $(seq 1 40); do s=$(admn $N GET /api/v1/domains/$DID/app/packages --label pk-st | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(b.get("phase"), b.get("script"), b.get("reason") or "", "|", " / ".join((b.get("lines") or [])[-3:])[:300])'); case "$s" in running*) sleep 6;; *) break;; esac; done; echo "   script: $s"
admn $N POST /api/v1/domains/$DID/app/restart '{}' --label restart | sbrief 200; sleep 6; echo "   site: $(site)"
echo "== 3c.3 SSH deploy with the deploy key (build on) $(ts)"
admn $N POST /api/v1/domains/$DID/git '{"repo_url":"git@git.lab.test:app.git","branch":"main","build":true}' --label git-set-ssh | sbrief 300
gitdeploy
admn $N POST /api/v1/domains/$DID/app/restart '{}' --label restart | sbrief 200; sleep 6; echo "   site: $(site)"
NRT=60 nrun n_git.sh log 6
grun $N g_nenv.sh $DID
gshn $N 'systemctl list-units --failed --no-legend --plain | cat; echo "(failed end)"'
echo "== c3node2 done $(ts)"
