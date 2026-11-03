# svcwait REQUEST_ID SECONDS : poll a service operation (read only) until it ends. sourced after lib.sh
svcwait() { local rid=$1 n=$(( ${2:-600} / 10 )) s i
  for i in $(seq 1 $n); do s=$(adm GET "/api/v1/service/operation?request_id=$rid" --label svcop | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); op=b.get("operation",b); print(op.get("status"), op.get("phase"), (op.get("error") or op.get("error_code") or "")[:200])' 2>&1)
    case "$s" in succeeded*|failed*|completed*) echo "  op $rid: $s ($(ts))"; return 0;; esac; sleep 10; done; echo "  op $rid still: $s"; return 1; }
