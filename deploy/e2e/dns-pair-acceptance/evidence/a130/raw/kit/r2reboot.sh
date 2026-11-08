# alpha.128 round 2: reboot of the joined server from its own panel (Maintenance), confirmed by request identity; it
# comes back and the pair stays healthy. Then (A7) the main 128 -> lab 129, one press: no Postfix .db re-hashed. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R2reboot-$CELL.log) 2>&1
raw() { python3 -I $A/raw.py $L $CELL backup "$@"; }
boot() { admn backup GET /api/v1/maintenance/clock --label clock 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]) if d.get("status")==200 else {}; print(b.get("boot_id",""), b.get("sync",""), b.get("time_zone",""))' 2>/dev/null; }
echo "== R.0 before $(ts)"; read B0 rest < <(boot); echo "   boot $B0 $rest"
echo "   without a request id:"; raw POST /api/v1/maintenance/reboot --json '{}' --show 200
RID=$(rid); T0=$(date +%s)
echo "   with a request id:"; raw POST /api/v1/maintenance/reboot --json '{}' --hdr X-CelikPanel-Request-Id:$RID --show 200
for i in $(seq 1 60); do sleep 5; b1 login $CELL backup >/dev/null 2>&1; read B1 rest < <(boot); [ -n "$B1" ] && [ "$B1" != "$B0" ] && { echo "   back after $(( $(date +%s) - T0 )) s: boot $B1 $rest"; break; }; done
sleep 30
gshn backup 'systemctl list-units --failed --all --no-legend --plain | cat; echo "(failed end)"; systemctl is-active celikpanel-panel celikpanel-agent nginx named 2>&1 | tr "\n" " "'
echo "-- the pair: DNS answers from both servers, the main's DNS page"
for ip in $MIP $BIP; do echo "  @$ip site1.$Z A: $(dig1 $ip site1.$Z A)  SOA $Z: $(dig1 $ip $Z SOA | cut -c1-60)"; done
admn main GET /api/v1/dns/overview --label dnsov | python3 $H/dnswatch.py | cut -c1-300
admn main GET /api/v1/setup/peer --label peer | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print("   peer", b.get("role"), json.dumps(b.get("inspection"))[:300])'
echo "== R.1 (A7) the main 128 -> lab 129 label, one press: Postfix maps not re-hashed $(ts)"
bash $A/portalF.sh latest v0.1.0-alpha.129
grun main g_snap.sh pre129 > /dev/null
CELL=r9 bash $A/upd.sh main
grun main g_snap.sh post129 pre129
gshn main 'systemctl list-units --failed --all --no-legend --plain | cat; echo "(failed end)"'
bash $A/portalF.sh latest v0.1.0-alpha.128
echo "== r2reboot done $(ts)"
