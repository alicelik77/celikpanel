# alpha.129 case 6 (b): token deletion whose DNS clean-up waits (joined named paused right after the delete starts),
# status address readable by the token, second press, resume -> gone. Also the pending domain of run (a). Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C6tok-$CELL.log) 2>&1
SUB=$(cat $T/sub-cust1.id)
raw() { python3 -I $A/raw.py $L $CELL main "$@"; }
st() { raw GET /api/v1/domains/$1/deletion-status --as none --bearer-file $T/tok-mg.txt --show 400 2>&1 | head -1 | cut -c1-400; }
watchgone() { local TD=$1 T0=$(date +%s) last="" s; while [ $(( $(date +%s) - T0 )) -lt ${2:-420} ]; do
  s=$(st $TD); [ "$s" != "$last" ] && { echo "   +$(( $(date +%s) - T0 ))s $s"; last=$s; }
  [ -z "$(domain_id $3 main)" ] && { echo "   GONE from the domain list after $(( $(date +%s) - T0 )) s"; return 0; }; sleep 15; done; echo "   still listed after ${2:-420} s"; }
echo "== 6.5 the pending domain of 6.2 (named running again): delete with the token $(ts)"
TD=$(domain_id tokpend.$Z main); echo "   tokpend id $TD"
RAW_TIMEOUT=300 raw DELETE "/api/v1/domains/$TD?confirm_domain=tokpend.$Z" --as none --bearer-file $T/tok-mg.txt --show 700
watchgone $TD 420 tokpend.$Z
echo "== 6.6 a fresh token domain; delete, and the joined named paused 1 s later $(ts)"
infra delegate tokdel2.$Z ns1.$Z $MIP ns2.$Z $BIP > /dev/null
RAW_TIMEOUT=400 raw POST /api/v1/domains/create --as none --bearer-file $T/tok-mg.txt --json "{\"domain\":\"tokdel2.$Z\",\"project_type\":\"static\",\"subscription_id\":$SUB}" --show 300
TD2=$(domain_id tokdel2.$Z main); echo "   tokdel2 id $TD2"; sleep 20
( RAW_TIMEOUT=300 raw DELETE "/api/v1/domains/$TD2?confirm_domain=tokdel2.$Z" --as none --bearer-file $T/tok-mg.txt --show 700 > $T/tokdel.out 2>&1 ) &
sleep 1; gshn backup 'kill -STOP $(pidof named); ps -o stat= -p $(pidof named)'
wait; cat $T/tokdel.out
echo "   status (token):"; st $TD2
echo "   second press:"; RAW_TIMEOUT=300 raw DELETE "/api/v1/domains/$TD2?confirm_domain=tokdel2.$Z" --as none --bearer-file $T/tok-mg.txt --show 700
sleep 45; echo "   status after 45 s:"; st $TD2
echo "== 6.7 named resumed $(ts)"; gshn backup 'kill -CONT $(pidof named); systemctl is-active named'
watchgone $TD2 420 tokdel2.$Z
echo "   @$MIP tokdel2 SOA: [$(dig1 $MIP tokdel2.$Z SOA)] @$BIP: [$(dig1 $BIP tokdel2.$Z SOA)]"
echo "== k6b done $(ts)"
