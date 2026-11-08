# case 1: fresh install of ${VER} through get.sh as the owner types it. env: S OS IPF. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh; source $H/common.sh
CELL=${S}f
exec > >(tee -a $LOGS/C1fresh-$S.log) 2>&1
echo "== provision $(ts) $OS $IPF"; T0=$(date +%s)
b1 provision $CELL main $OS $IPF frankfurt 3072 > $LOGS/prov-$CELL-main.log 2>&1; tail -c 250 $LOGS/prov-$CELL-main.log; echo
sethost $CELL main host1.$S.test
bash $A/portalF.sh trust $CELL main
echo "== get.sh in a root terminal $(ts)"; T1=$(date +%s)
A114_PORTAL=$B/portal-${VER:-v0.1.0-alpha.129} timeout 4000 python3 -I $A/getsh_install.py $L $CELL main > $LOGS/install-$CELL-main.log 2>&1; echo "install exit $? after $(( $(date +%s) - T1 )) s"
python3 $H/insum.py $LOGS/install-$CELL-main.log main
bash $A/portalF.sh trust $CELL main
b1 login $CELL main | python3 -c 'import json,sys; d=json.load(sys.stdin); print("login", d["license_after"].get("state"), "can_use_panel", d["access"].get("can_use_panel"))'
b1 api $CELL main GET /api/v1/panel/version --label version --limit 2000 | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["status"], d["body"][:300])'
lab ssh $CELL main -- "sudo -n tee /root/g_mig.sh >/dev/null" < $A/g_mig.sh; lab ssh $CELL main -- "sudo -n bash /root/g_mig.sh" | head -2
b1 sh $CELL main "cat /etc/os-release | grep PRETTY; systemctl list-units --failed --all --no-legend --plain | cat; echo '(failed end)'; cat /opt/celikpanel/release.commit; grep -E '^CELIKPANEL_LISTEN' /etc/celikpanel/panel.env; systemctl is-active celikpanel-panel celikpanel-agent | tr '\n' ' '" --label state | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
echo "== fresh $S done $(ts)"
