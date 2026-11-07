# d12 defect: evidence, then the owner path (Components -> start BIND) and the setup verification. Lab only.
export S=d12 IPU=192.0.3.220
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/hdr.sh 2>/dev/null
exec > >(tee -a $LOGS/D1-bind-d12.log) 2>&1
echo "== evidence $(ts)"; grun main g_named.sh
opnow $CELL main | head -2 | cut -c1-600
echo "== service status for named $(ts)"; admn main GET "/api/v1/service/status?name=named" --label st | sbrief 600
echo "== owner: Components -> Start BIND $(ts)"; admn main POST /api/v1/service/action "{\"name\":\"named\",\"action\":\"start\"}" --label act --timeout 120 | sbrief 800
gshn main "systemctl is-active named"
for i in $(seq 1 12); do sleep 15; s=$(opnow $CELL main | head -1 | cut -c1-120); echo "$(ts) $s"; case "$s" in succeeded*|failed*) break;; esac; done
gshn main "systemctl is-active named; journalctl -u celikpanel-agent --since -4min -o cat --no-pager | grep -iE \"bind|named\" | tail -5 | cut -c1-400"
