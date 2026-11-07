# alpha.128 case 7: fresh Debian 13 (cell f8, 192.0.3.194) through get.sh as the owner types it, from the lab portal of 128. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit
source $A/env.sh; source $H/common.sh
exec > >(tee -a $LOGS/C7-f8.log) 2>&1
echo "== provision $(ts)"; b1 provision f8 main debian13 192.0.3.194 single 3072 > $LOGS/prov-f8-main.log 2>&1; tail -c 300 $LOGS/prov-f8-main.log; echo
sethost f8 main host1.f8.test
bash $A/portalF.sh trust f8 main
A114_PORTAL=$B/portal-v0.1.0-alpha.128 timeout 4000 python3 -I $A/getsh_install.py $L f8 main > $LOGS/install-f8-main.log 2>&1; echo "install exit $?"
python3 $H/insum.py $LOGS/install-f8-main.log main
bash $A/portalF.sh trust f8 main
b1 login f8 main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("login", d["license_after"].get("state"), "can_use_panel", d["access"].get("can_use_panel"))'
b1 api f8 main GET /api/v1/panel/version --label version --limit 2000 | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["status"], d["body"][:400])'
b1 sh f8 main "systemctl list-units --failed --all --no-legend --plain | cat; echo '(failed end)'; cat /opt/celikpanel/release.commit; grep -E '^CELIKPANEL_LISTEN' /etc/celikpanel/panel.env" --label f8-state | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
echo "== case7 done $(ts)"
