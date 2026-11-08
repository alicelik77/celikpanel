# alpha.130 removal retest on one fresh cell (u1u Ubuntu 24.04 / d1u Debian 13, set up at 129, updated to the new
# head first): whole PostgreSQL removal, Postfix removal (owner drop-in + owner file), Dovecot removal, Postfix
# reinstall. usage: rt.sh S IP. Lab only.
S=$1; export CELL=${S}u IP=$2 MIP=$2 BIP=$2 Z=$S.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
export CELL=${S}u IP=$2 MIP=$2 BIP=$2 Z=$S.test
exec > >(tee -a $LOGS/RT-$CELL.log) 2>&1
b1 login $CELL main > /dev/null 2>&1
look() { grun main g_rt.sh "$@"; }
rm1() { echo "-- remove $1 (together: ${2:-none}) $(ts)"; admn main GET "/api/v1/service/removal-impact?service_id=$1" --label impact-$1 | sbrief 500
  local R=$(rid) body="{\"service_id\":\"$1\",\"request_id\":\"$R\"}"
  [ -n "${2:-}" ] && body="{\"service_id\":\"$1\",\"request_id\":\"$R\",\"remove_together\":$2}"
  for i in $(seq 1 20); do r=$(python3 -I $A/raw.py $L $CELL main POST /api/v1/service/uninstall --as admin --json "$body" --hdr Content-Type:application/json --hdr X-CelikPanel-Request-Id:$R --show 1800); echo "$r" | grep -qE "HOST_MUTATION_BUSY|service_operation_busy" || break; sleep 20; done; echo "$r"
  echo "$r" | grep -qE '^ *202' && { svcwaitn main $R 1200; admn main GET "/api/v1/service/operation?request_id=$R" --label rmop | sbrief 1800; }; }
echo "== $CELL $(ts)"; ver main; look all
rm1 postfix '["rspamd","roundcube"]'; sleep 5; look postfix
rm1 dovecot; sleep 5; look dovecot
echo "-- Postfix again from Components $(ts)"; installc main postfix; sleep 5; look postfix
echo "== rt $CELL done $(ts)"
