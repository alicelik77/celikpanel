source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
adm GET /api/v1/setup/operation --label op > $T/op.json
python3 -c '
import json,sys
b=json.loads(json.load(open(sys.argv[1]))["body"]); op=b.get("operation",b)
print({k:v for k,v in op.items() if k not in ("steps",)})
for s in op.get("steps",[])[:1]: print(json.dumps(s)[:1500])
' $T/op.json
cat > $T/g.sh <<'G'
journalctl --no-pager --since "13:59:50" --until "14:00:40" 2>/dev/null | grep -vE "sshd|systemd-logind|sudo|pam_unix|session" | tail -40 | cut -c1-240
G
cp $T/g.sh $A/g_tmp.sh; gq g_tmp.sh
