source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
T=$B/t-r9; ls $B | head; ls -d $B/*r9* 2>/dev/null
for f in $(ls -d $B/*/plans.json 2>/dev/null); do echo $f; python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("plans") if isinstance(b,dict) else b
for p in l: print(p.get("name"), json.dumps(p.get("rights"))[:600])' $f; done
cat $(ls -d $B/*/planrights.json | head -1)
