# alpha.130 round, case 1: A (main) 129 -> 130 with one forced failure first (rolls back), B (joined) pressed once.
# Migrations, units, native files, sites. Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C1upd-$CELL.log) 2>&1
for n in backup main; do grun $n g_mig.sh; done
echo "== B: press Update once $(ts)"
bash $A/portalF.sh latest v0.1.0-alpha.130
CELL=s0 bash $A/upd.sh backup
echo "== A: forced failure, rollback, second press $(ts)"
CELL=s0 N=main bash $A/rb_s0.sh
for n in backup main; do echo "== $n after $(ts)"; grun $n g_mig.sh; grun $n g_snap.sh post130 pre129
  gshn $n 'systemctl list-units --failed --all --no-legend --plain | cat; echo "(failed end)"; cat /opt/celikpanel/release.commit; systemctl is-active celikpanel-panel celikpanel-agent nginx'; done
echo "-- sites answer after the update"
for d in site1 cust1 bsite; do ip=$MIP; [ $d = bsite ] && ip=$BIP; echo "  $d.$Z: $(lab ssh beta1-lab labnet -- "curl -sS -m 10 -o /dev/null -w '%{http_code}' -H 'Host: $d.$Z' http://$ip/" 2>/dev/null)"; done
echo "== c1upd done $(ts)"
