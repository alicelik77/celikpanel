# alpha.130 round, item 4 (A, Debian 13): panel TLS audit rows, SSH status, monitoring. Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/K4ops-$CELL.log) 2>&1
b1 login $CELL main > /dev/null 2>&1
adm GET /api/v1/security/audit --label audit --timeout 120 | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"])
def walk(x):
    if isinstance(x,dict):
        t=json.dumps(x).lower()
        if ("tls" in t or "certificate" in t) and ("status" in x or "state" in x or "result" in x) and len(t)<1500: print("  ", json.dumps(x, ensure_ascii=False)[:700]); return
        for v in x.values(): walk(v)
    elif isinstance(x,list):
        for v in x: walk(v)
print(d["status"]); walk(b)'
echo "-- ssh status"; adm GET /api/v1/security/ssh-password-login --label ssh | sbrief 1200
echo "-- os updates summary"; adm GET /api/v1/security/os-updates/summary --label osu | sbrief 900
gshn main 'echo | openssl s_client -connect 127.0.0.1:1923 -servername frankfurt.s0.test 2>/dev/null | openssl x509 -noout -issuer -ext subjectAltName | tr "\n" " "'
