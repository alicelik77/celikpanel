# alpha.127 case 5 (database form, joined server pg7.test): rule up front, meter, generate/show/hide/copy, submit off
# until the rule is met, a server refusal said once; the API refuses a weak password (PASSWORD_WEAK). Lab only.
export CELL=r7 IP=192.0.3.191 Z=r7.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a127lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=backup
exec > >(tee -a $LOGS/C5-$CELL-$N.log) 2>&1
admn $N PUT /api/v1/me/guide-state '{"tour_version":1,"tour_state":"skipped"}' --label tour-skip | sbrief 200
python3 $A/uiitems.py pw-db pg7.test $T/pwdb.json pwweak
timeout 1500 python3 -I $H/shots.py $CELL $N c5db2 $T/pwdb.json --langs en,tr --widths desktop | python3 $A/shotsum.py
echo "== API: weak database password $(ts)"
admn $N POST /api/v1/domains/1/databases '{"name":"weak127","type":"postgresql","password":"[REDACTED]"}' --label db-weak | sbrief 600
admn $N POST /api/v1/domains/1/databases '{"name":"weak127","type":"postgresql","password":"[REDACTED]"}' --label db-weak2 | sbrief 600
admn $N GET /api/v1/domains/1/databases --label dbs | sbrief 600
echo "== c5db done $(ts)"
