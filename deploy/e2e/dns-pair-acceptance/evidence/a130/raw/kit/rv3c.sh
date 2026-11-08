# alpha.130 re-verify: while the boot is held: delete rule 5 by its id, read the panel's start state. Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R3a-$CELL.log) 2>&1
echo "== $(ts) held boot, direct id"; grun main g_k2.sh state
as cust1 DELETE /api/v1/database-remote/rules/5 | sbrief 500; T1=$(date +%s)
for i in $(seq 1 14); do s=$(grun main g_k2.sh state); echo "$s" | grep -q "3306 ip saddr 192.0.3.1" || { echo "   3306 address gone after $(( $(date +%s) - T1 )) s"; break; }; sleep 5; done; echo "$s"
adm GET /api/v1/panel/version --label ver | sbrief 300
gshn main 'journalctl -u celikpanel-panel --since -4min -o cat --no-pager | grep -iE "start|systemd|ready|waiting" | tail -8 | cut -c1-220'
