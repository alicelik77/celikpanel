source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
b1 api aru main GET /api/v1/operation-progress/4921c32e5316739e53c54dd284fc520f --label opp --limit 20000 | python3 -c "import json,sys; d=json.load(sys.stdin); print(d[\"status\"], d[\"body\"][:2500])"
lab ssh aru main -- "sudo -n python3 -c \"import sqlite3; c=sqlite3.connect(\\\"file:/var/lib/celikpanel/celikpanel.db?mode=ro\\\",uri=True); print([r[0] for r in c.execute(\\\"select name from sqlite_master where name like \\\\\\\"%oper%\\\\\\\"\\\")])\""
