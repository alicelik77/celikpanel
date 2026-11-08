# B lane item 5: PostgreSQL on the joined server after 130. usage: b_pg.sh PART... (cfg inst16 rm16 port variant archive state). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/b_lib.sh
exec > >(tee -a $LOGS/B5pg-$CELL.log) 2>&1
cfg() { admn $N GET /api/v1/managed-services --label ms > $T/ms-b.json; python3 -c '
import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); rows=b if isinstance(b,list) else (b.get("services") or b.get("items") or [])
for r in rows:
    if r.get("id")=="postgresql":
        print("  postgresql row:", json.dumps({k:r.get(k) for k in ("status","version","package","installed_package","cluster","clusters","notice","kept")})[:900])
        for f in r.get("config_files") or []: print("   config file:", f.get("path") if isinstance(f,dict) else f)' $T/ms-b.json; }
clusters() { admn $N GET /api/v1/service/postgresql/clusters --label pgclusters > $T/pgc.json; python3 -c '
import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"])
print("  clusters", d["status"])
for c in b.get("clusters") or []: print("   ", json.dumps({k:c.get(k) for k in ("version","name","port","status","online","databases","leftover","config_path")},ensure_ascii=False)[:700])' $T/pgc.json; }
for part in "$@"; do case $part in
state) echo "== state $(ts)"; grun $N g_pgl.sh; clusters; cfg ;;
cfg) echo "== (a) Components reads/edits 18's files $(ts)"; cfg; clusters ;;
scan) echo "== Components scan (the page's Rescan) $(ts)"; admn $N POST /api/v1/managed-services/scan "{}" --label scan --timeout 300 | python3 -c "import json,sys; print(json.load(sys.stdin)[\"status\"])"; cfg ;;
inst16) echo "== (b) install PostgreSQL 16 through the panel (pgdg) $(ts)"; installc $N postgresql postgresql-16; grun $N g_pgl.sh; clusters ;;
rm16) echo "== (b) remove version 16 through the panel $(ts)"
  admn $N GET "/api/v1/service/removal-impact?service_id=postgresql&package=postgresql-16" --label impact16 | sbrief 1200
  R=$(rid); admn $N POST /api/v1/service/uninstall "{\"service_id\":\"postgresql\",\"package\":\"postgresql-16\",\"request_id\":\"$R\"}" --label rm16 --timeout 900 | sbrief 900
  sleep 5; grun $N g_pgl.sh; clusters; audit 'postgresql' 30 ;;
port) echo "== (c) move 18 to 5432 $(ts)"
  admn $N GET "/api/v1/service/postgresql/clusters/port-preview?version=18&name=main&port=5432" --label portprev > $T/pp.json; sbrief 1200 < $T/pp.json
  SHA=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("preview") or {}).get("config_sha256",""))' $T/pp.json)
  admn $N POST /api/v1/service/postgresql/clusters/port "{\"version\":\"18\",\"name\":\"main\",\"port\":5432,\"expected_sha256\":\"$SHA\"}" --label port --timeout 600 | sbrief 900
  sleep 3; grun $N g_pgl.sh; clusters
  gshn $N 'cd /tmp; ss -ltnp | grep -E ":543[23] " | sed "s/  */ /g"; sudo -u postgres psql -p 5432 -At -c "select datname from pg_database where datname like '"'"'pgb%'"'"'"'
  admn $N GET /api/v1/domains/$(cat $T/pgb.id)/databases --label dbs | sbrief 600 ;;
variant) echo "== (d) 16 again, owner data in it, owner apt-get remove $(ts)"
  [ -z "${SKIPINST:-}" ] && installc $N postgresql postgresql-16; grun $N g_pgl.sh
  grun $N g_pgown.sh
  echo "-- trigger the cleanup: restart the panel (lab) $(ts)"; gshn $N 'systemctl restart celikpanel-panel; sleep 5; systemctl is-active celikpanel-panel'
  for i in $(seq 1 30); do sleep 10; b1 login $CELL $N >/dev/null 2>&1; gshn $N 'journalctl -u celikpanel-panel --since -4min -o cat --no-pager | grep -ciE "leftover"' | tail -1 | grep -q '^[1-9]' && break; done
  gshn $N 'journalctl -u celikpanel-panel -u celikpanel-agent --since -6min -o cat --no-pager | grep -iE "leftover" | tail -6 | cut -c1-300'
  grun $N g_pgl.sh; clusters ;;
archive) echo "== (d) Archive and remove the kept 16/main $(ts)"
  R=$(rid); admn $N POST /api/v1/service/postgresql/clusters/leftover "{\"version\":\"16\",\"name\":\"main\",\"request_id\":\"$R\"}" --label leftover-rm --timeout 900 | sbrief 1200
  sleep 3; grun $N g_pgl.sh; clusters; audit 'postgresql' 30 ;;
esac; done
echo "== b_pg $* done $(ts)"
