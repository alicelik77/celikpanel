# alpha.128 case 3b (HTTPS): the lab Git server switched to smart HTTP (git http-backend), HTTPS deploy again. Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C3node-$CELL.log) 2>&1
N=main; D=node8.$Z; DID=$(cat $T/node8.id)
site() { lab ssh beta1-lab labnet -- "curl -sS -m 10 -H 'Host: $D' http://$MIP/" 2>/dev/null | head -c 400; }
gitdeploy() { local R="git-$(rid)"; admn $N POST /api/v1/domains/$DID/git/deploy "{\"request_id\":\"$R\"}" --label git-deploy --timeout 300 | sbrief 300
  for i in $(seq 1 50); do s=$(admn $N GET /api/v1/domains/$DID/git --label git-st | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); s=b.get("status") or {}; print(s.get("phase"), s.get("reason") or "", (s.get("error") or "")[:200], "|", " / ".join((s.get("lines") or [])[-5:])[:600])'); case "$s" in running*) sleep 6;; *) break;; esac; done; echo "   deploy: $s"; }
echo "== 3d.0 smart HTTP on the lab Git server $(ts)"
lab ssh beta1-lab labnet -- "sudo -n tee /root/git-smart.py >/dev/null" < $A/n_gitsmart.py
NRT=60 nrun n_git.sh commit https-3
lab ssh beta1-lab labnet -- "sudo -n sh -c 'systemctl stop a128-git; systemd-run --unit a128-git -p RuntimeMaxSec=28800 python3 -I /root/git-smart.py >/dev/null 2>&1; sleep 1; systemctl is-active a128-git'"
echo "== 3d.1 HTTPS deploy (build on) $(ts)"
admn $N POST /api/v1/domains/$DID/git '{"repo_url":"https://git.lab.test/app.git","branch":"main","build":true}' --label git-set | sbrief 200
gitdeploy
admn $N POST /api/v1/domains/$DID/app/restart '{}' --label restart | sbrief 100; sleep 6; echo "   site: $(site)"
echo "== 3d.2 a package script (build) $(ts)"
admn $N POST /api/v1/domains/$DID/app/packages "{\"request_id\":\"node-$(rid)\",\"build\":false,\"script\":\"build\"}" --label pk-run --timeout 600 | sbrief 200
for i in $(seq 1 40); do s=$(admn $N GET /api/v1/domains/$DID/app/packages --label pk-st | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(b.get("phase"), b.get("script"), b.get("reason") or "", "|", " / ".join((b.get("lines") or [])[-3:])[:300], "| scripts", b.get("scripts"))'); case "$s" in running*) sleep 6;; *) break;; esac; done; echo "   script: $s"
admn $N POST /api/v1/domains/$DID/app/restart '{}' --label restart | sbrief 100; sleep 6; echo "   site: $(site)"
NRT=60 nrun n_git.sh log 4
echo "== c3node3 done $(ts)"
