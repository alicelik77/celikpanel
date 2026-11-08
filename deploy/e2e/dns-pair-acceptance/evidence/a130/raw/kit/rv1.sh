# alpha.130 re-verify round on the final candidate (lab 131 = 899b40b1f + identity bump): B then A, each a forced
# failure first (rolls back), then the owner's press; A has one MariaDB and one PostgreSQL rule; right after A's start
# the PostgreSQL rule is deleted (within 3 minutes). Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R1upd-$CELL.log) 2>&1
b1 login $CELL main > /dev/null 2>&1; b1 login $CELL backup > /dev/null 2>&1
echo "== rules on A before $(ts)"; grun main g_k2.sh nft
echo "== B (no remote-access rules) $(ts)"; CELL=s0 N=backup bash $A/rb2.sh
date -u +%s > $T/b-start.epoch
echo "== A (one MariaDB + one PostgreSQL rule) $(ts)"; CELL=s0 N=main bash $A/rb2.sh
TS=$(date +%s)
echo "== A: delete the PostgreSQL rule right after the start $(ts)"
RP=$(as cust1 GET /api/v1/database-remote/databases/$(cat $T/rdb-postgresql.id) | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=[r for r in (b.get("rules") or []) if r.get("address")=="192.0.3.1"]; print(l[0]["id"] if l else "")')
echo "   rule $RP; panel up for $(( $(date +%s) - TS )) s since the update answer"
as cust1 DELETE /api/v1/database-remote/rules/$RP | sbrief 400; T1=$(date +%s)
for i in $(seq 1 20); do s=$(grun main g_k2.sh nft | grep -c "5432 ip saddr 192.0.3.1"); [ "$s" = 0 ] && { echo "   5432 address gone from celikpanel_dbremote after $(( $(date +%s) - T1 )) s"; break; }; sleep 5; done
[ "$s" != 0 ] && echo "   STILL in the filter after $(( $(date +%s) - T1 )) s"
grun main g_k2.sh nft
for n in backup main; do echo "== $n after $(ts)"; grun $n g_mig.sh | head -1
  gshn $n 'systemctl list-units --failed --all --no-legend --plain | cat; echo "(failed end)"; systemctl is-active celikpanel-panel celikpanel-agent nginx | tr "\n" " "'; ver $n; done
echo "-- sites answer after the update"
for d in site1 cust1 bsite; do ip=$MIP; [ $d = bsite ] && ip=$BIP; echo "  $d.$Z: $(lab ssh beta1-lab labnet -- "curl -sS -m 10 -o /dev/null -w '%{http_code}' -H 'Host: $d.$Z' http://$ip/" 2>/dev/null)"; done
echo "== rv1 done $(ts)"
