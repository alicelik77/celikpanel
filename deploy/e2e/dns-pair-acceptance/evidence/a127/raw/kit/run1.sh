# alpha.127 case 1: fresh Debian 13 (cell f7, 192.0.3.194), the owner's get.sh from the candidate's lab portal. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a127lab/kit
source $A/env.sh; source $H/common.sh
exec > >(tee -a $LOGS/C1-f7.log) 2>&1
echo "== provision $(ts)"; b1 provision f7 main debian13 192.0.3.194 single 3072 > $LOGS/prov-f7-main.log 2>&1; tail -c 300 $LOGS/prov-f7-main.log; echo
sethost f7 main host1.f7.test
bash $A/portalF.sh trust f7 main
A114_PORTAL=$B/portal-v0.1.0-alpha.127 timeout 4000 python3 -I $A/c1_install.py $L f7 main
b1 login f7 main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("login", d["license_after"].get("state"), "can_use_panel", d["access"].get("can_use_panel"))'
b1 api f7 main GET /api/v1/panel/version --label version --limit 2000 | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["status"], d["body"][:400])'
echo "== case1 done $(ts)"
