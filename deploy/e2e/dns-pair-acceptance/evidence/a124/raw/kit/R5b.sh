# alpha.124 case 5 (second probe set) on p124 main: endpoints that call the Agent live. Lab only.
export CELL=p124 IP=192.0.3.190 Z=p124.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
exec > >(tee -a $LOGS/R5b-$CELL.log) 2>&1
DID=$(cat $T/did6)
probe() { echo "  -- $1"
  adm GET /api/v1/postfix/queue --label queue-$1 | sbrief 500
  adm GET /api/v1/mail/passwordless --label pwmail-$1 --timeout 90 | sbrief 500
  adm GET "/api/v1/service/candidate?id=redis" --label cand-$1 | sbrief 500
  adm GET "/api/v1/service/install-preview?step=redis" --label prev-$1 --timeout 150 | sbrief 500
  adm GET /api/v1/domains/$DID/dnssec --label dnssec-$1 | sbrief 500
  adm POST /api/v1/domains/$DID/dnssec --label dnssec-post-$1 | sbrief 500; }
echo "== 5b.0 before $(ts)"; probe before
echo "== 5b.1 the Agent stopped (lab load) $(ts)"
gsh1 'systemctl stop celikpanel-agent; systemctl is-active celikpanel-agent'
probe stopped
echo "== 5b.2 the Agent started, its socket made root:root (lab load) $(ts)"
gsh1 'systemctl start celikpanel-agent; sleep 8; chown root:root /run/celikpanel/agent.sock; stat -c "%A %U:%G %n" /run/celikpanel/agent.sock'
probe denied
echo "== 5b.3 the owner runs sudo systemctl restart celikpanel-agent $(ts)"
gsh1 'systemctl restart celikpanel-agent; sleep 10; systemctl is-active celikpanel-agent; stat -c "%A %U:%G %n" /run/celikpanel/agent.sock'
probe after
gsh1 'journalctl -u celikpanel-panel --since "-5min" -o cat --no-pager | grep -E "\[503\]|\[500\]|\[502\]" | cut -c1-260 | tail -12'
echo "== R5b done $(ts)"
