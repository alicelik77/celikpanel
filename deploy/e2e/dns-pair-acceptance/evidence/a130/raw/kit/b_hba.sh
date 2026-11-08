# B lane item 5e: a pg_hba.conf save without the local administrator line is refused and names the line. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/b_lib.sh
exec > >(tee -a $LOGS/B5hba-$CELL.log) 2>&1
F=/etc/postgresql/18/main/pg_hba.conf
admn $N GET "/api/v1/config?path=$F" --label hba-read > $T/hba.json
python3 - $T/hba.json $T/hba-save.json $F <<'P'
import json,sys
d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); print("read", d["status"], sorted(b.keys()))
c=b.get("Content",""); lines=c.split("\n")
keep=[l for l in lines if not (l.strip().startswith("local") and l.split()[2:3]==["postgres"] and "peer" in l)]
print("removed lines:", [l for l in lines if l not in keep])
json.dump({"path":sys.argv[3],"content":"\n".join(keep),"version":b.get("Version","")},open(sys.argv[2],"w"))
P
echo "-- save without the local postgres peer line"
admn $N POST /api/v1/config @$T/hba-save.json --label hba-save | sbrief 1200
rm -f $T/hba-save.json
gshn $N "grep -n \"^local\" $F | head -4"
