# case 2: the owner presses Update once (127 -> 128); time, units, schema, native files. env: S IPU. Lab only.
export CELL=${CELLX:-${S}u} IP=$IPU MIP=$IPU BIP=$IPU Z=$S.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
export CELL=${CELLX:-${S}u} IP=$IPU MIP=$IPU BIP=$IPU Z=$S.test
exec > >(tee -a $LOGS/C2upd-$S.log) 2>&1
grun main g_mig.sh | head -1
CELL=$CELL IP=$IP Z=$Z bash $A/upd.sh main
echo "== after $(ts)"; grun main g_mig.sh; grun main g_snap.sh post128 pre127
gshn main 'systemctl list-units --failed --all --no-legend --plain | cat; echo "(failed end)"; cat /opt/celikpanel/release.commit; systemctl is-active celikpanel-panel celikpanel-agent nginx postfix dovecot mariadb | tr "\n" " "'
for d in site1 cust1; do echo "  $d.$Z: $(lab ssh beta1-lab labnet -- "curl -sS -m 10 -o /dev/null -w '%{http_code}' -H 'Host: $d.$Z' http://$IP/" 2>/dev/null)"; done
echo "== upd128 $S done $(ts)"
