source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
b1 api d12u main GET /api/v1/dns/overview --label ov --limit 400000 | python3 -c "import json,sys; d=json.load(sys.stdin); b=json.loads(d[\"body\"]); print(json.dumps({k:v for k,v in b.items() if k not in (\"names\",\"mail\")})[:2500])"
b1 api d12u main GET /api/v1/hosting/capabilities --label caps --limit 100000 | python3 -c "import json,sys; d=json.load(sys.stdin); b=json.loads(d[\"body\"]); print({k:v for k,v in b.items() if \"dns\" in k})"
