# once: copy a large site (about 2.4 GB, 40 000 small files + 22 large ones, MariaDB ~330 MB) onto a site that
# already has content: refused without overwrite, then with overwrite: safety backup, new database, files. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/hdr.sh
exec > >(tee -a $LOGS/C9copy-$S.log) 2>&1
echo "== 9.0 source big.$Z and target tgt.$Z $(ts)"
BIG=$(mksite main big.$Z php); echo "big id $BIG"; echo $BIG > $T/big.id
TGT=$(mksite main tgt.$Z php); echo "tgt id $TGT"; echo $TGT > $T/tgt.id
mkcred dbbig; admn main POST /api/v1/domains/$BIG/databases "{\"name\":\"bigdb\",\"type\":\"mysql\",\"password\":\"$(pw dbbig)\"}" --label db-big --timeout 300 | sbrief 300
admn main GET /api/v1/domains/$BIG/databases --label dbs > $T/bigdbs.json
read DBID DBN < <(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("databases") if isinstance(b,dict) else b; x=l[0]; print(x["id"], x["name"])' $T/bigdbs.json); echo "database $DBN id $DBID"
GQT=3000 grun main g_big.sh fill $BIG $DBN
grun main g_big.sh sizes $TGT
echo "== 9.1 what the copy window offers $(ts)"; adm GET /api/v1/domains/$BIG/copy --label copy-get | sbrief 800
echo "== 9.2 copy without overwrite onto a site with content $(ts)"
adm POST /api/v1/domains/$BIG/copy "{\"target_domain_id\":$TGT,\"database_id\":$DBID,\"overwrite\":false}" --label copy-no --timeout 600 | sbrief 600
echo "== 9.3 copy with overwrite $(ts)"; T0=$(date +%s)
adm POST /api/v1/domains/$BIG/copy "{\"target_domain_id\":$TGT,\"database_id\":$DBID,\"overwrite\":true}" --label copy-yes --timeout 5400 | sbrief 1200
echo "   answer after $(( $(date +%s) - T0 )) s"
grun main g_big.sh sizes $TGT
admn main GET /api/v1/domains/$TGT/databases --label tdbs | sbrief 600
CN=$(admn main GET /api/v1/domains/$TGT/databases --label tdbs | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("databases") if isinstance(b,dict) else b; print(l[0]["name"] if l else "")')
[ -n "$CN" ] && gshn main "mariadb -N -B -e \"select count(*) from $CN.pages\""
echo "   target backups:"; admn main GET /api/v1/domains/$TGT/backups --label tbk | sbrief 800
echo "   the copied site answers: $(lab ssh beta1-lab labnet -- "curl -sS -m 20 -H 'Host: tgt.$Z' http://$IP/" 2>/dev/null | head -c 80)"
gshn main 'df -h / | tail -1; systemctl list-units --failed --no-legend --plain | cat; echo "(failed end)"'
echo "== k9copy $S done $(ts)"
