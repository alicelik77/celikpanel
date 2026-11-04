source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R8fallback-$CELL.log) 2>&1
echo "== after the broken redeploy $(ts)"
gsh1 'curl -sk -o /tmp/fb.out -w "flask https %{http_code}\n" --resolve flask119.test:443:127.0.0.1 https://flask119.test/; head -c 200 /tmp/fb.out | tr "\n" " "; echo; systemctl is-active celikrt-1.service; systemctl show celikrt-1.service -p ExecStart --value | head -c 300; echo'
as cust119b GET /api/v1/domains/$(cat $T/id-flask119.test)/runtime-apps | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); a=b.get("app") or {}; s=a.get("status") or {}; print("  app state", a.get("state"), "active", s.get("active_state"), "serving", s.get("serving"), "states", a.get("states"))'
as cust119b GET "/api/v1/domains/$(cat $T/id-flask119.test)/runtime-apps/logs?lines=20" | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("   ", x[:200]) for x in (b.get("lines") or [])[-10:]]'
