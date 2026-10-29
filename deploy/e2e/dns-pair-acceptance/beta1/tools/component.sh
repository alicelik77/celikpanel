# usage: component_install CELL NODE SERVICE   (owner path: Components preview, confirm the listed packages, install, wait)
component_install() {
  local cell=$1 node=$2 svc=$3
  $B api $cell $node GET "/api/v1/service/install-preview?step=$svc" --label components-preview-$svc --limit 60000 --timeout 120 > /tmp/prev-$cell-$node.json
  local RID=$(python3 -c 'import secrets; print(secrets.token_hex(16))')
  python3 - /tmp/prev-$cell-$node.json $svc $RID > /tmp/inst-$cell-$node.json <<'PY'
import json, sys
d = json.load(open(sys.argv[1])); b = json.loads(d["body"])
print(json.dumps({"service_id": sys.argv[2], "check_installs": True, "confirmed_installs": [n["package"] for n in b["notable"]], "request_id": sys.argv[3]}))
sys.stderr.write("preview %s notable=%s refusals=%s\n" % (b["state"], [n["package"] for n in b["notable"]], b["refusals"]))
PY
  $B api $cell $node POST /api/v1/service/install @/tmp/inst-$cell-$node.json --label components-install-$svc --limit 600 | head -c 300; echo
  $B poll $cell $node "/api/v1/service/operation?request_id=$RID" 900 5 --until operation.status=succeeded --label component-op-$svc --brief 600 | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["reads"], d["until_met"], d["last"][:600])'
}
