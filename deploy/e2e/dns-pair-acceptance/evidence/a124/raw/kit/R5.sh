# alpha.124 case 5 on p124 main: the Agent not running, then its socket refused, then restarted. Lab only.
export CELL=p124 IP=192.0.3.190 Z=p124.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
exec > >(tee -a $LOGS/R5-$CELL.log) 2>&1
DID=$(cat $T/did6)
probe() { echo "  -- $1"; adm GET /api/v1/domains/$DID/dnssec --label dnssec-$1 | sbrief 900; adm GET /api/v1/domains/$DID/dns/records --label records-$1 | sbrief 500; adm GET "/api/v1/service/install-plan?id=redis" --label plan-$1 | sbrief 500; adm GET /api/v1/managed-services --label ms-$1 | sbrief 300; }
echo "== 5.0 before $(ts)"; probe before
echo "== 5.1 the Agent stopped (lab load: systemctl stop celikpanel-agent) $(ts)"
gsh1 'systemctl stop celikpanel-agent; systemctl is-active celikpanel-agent; ls -la /run/celikpanel/ | grep sock'
probe stopped
echo "== 5.2 the Agent started, then its socket made root:root (as a hand-started second Agent used to) $(ts)"
gsh1 'systemctl start celikpanel-agent; sleep 8; stat -c "%A %U:%G %n" /run/celikpanel/agent.sock; chown root:root /run/celikpanel/agent.sock; stat -c "%A %U:%G %n" /run/celikpanel/agent.sock'
probe denied
echo "== 5.3 the owner runs sudo systemctl restart celikpanel-agent $(ts)"
gsh1 'systemctl restart celikpanel-agent; sleep 10; systemctl is-active celikpanel-agent; stat -c "%A %U:%G %n" /run/celikpanel/agent.sock'
probe after
gsh1 'journalctl -u celikpanel-panel --since "-6min" -o cat --no-pager | grep -E "\[503\]|\[500\]" | cut -c1-300 | tail -6'
echo "== R5 done $(ts)"
