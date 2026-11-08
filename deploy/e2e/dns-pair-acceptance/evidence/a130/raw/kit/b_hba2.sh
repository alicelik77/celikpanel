# B lane item 5e (2): a save without any local rule is refused (banner names the rule); then the postgres line back. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/b_lib.sh
exec > >(tee -a $LOGS/B5hba-$CELL.log) 2>&1
F=/etc/postgresql/18/main/pg_hba.conf
save() { admn $N GET "/api/v1/config?path=$F" --label hba-read > $T/hba.json
python3 - $T/hba.json $T/hba-save.json $F $1 <<'P'
import json,sys
d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); c=b.get("Content",""); lines=c.split("\n")
if sys.argv[4]=="badrule":
    keep=lines[:]; keep.insert(len(keep)-1 if keep and keep[-1]=="" else len(keep), "host    all             all             999.1.1.1/32            scram-sha-256")
elif sys.argv[4]=="nolocal":
    keep=[l for l in lines if not l.strip().startswith("local")]
else:
    keep=[]
    for l in lines:
        if l.startswith("local   all             all") and not any(x.startswith("local   all             postgres") for x in keep): keep.append("local   all             postgres                                peer")
        keep.append(l)
print("lines", len(lines), "->", len(keep))
json.dump({"path":sys.argv[3],"content":"\n".join(keep),"version":b.get("Version","")},open(sys.argv[2],"w"))
P
admn $N POST /api/v1/config @$T/hba-save.json --label hba-save-$1 | sbrief 1500; rm -f $T/hba-save.json; }
echo "== $(ts) save with an invalid address rule"; save badrule
gshn $N "grep -n \"^local\" $F; ls /etc/postgresql/18/main/"
