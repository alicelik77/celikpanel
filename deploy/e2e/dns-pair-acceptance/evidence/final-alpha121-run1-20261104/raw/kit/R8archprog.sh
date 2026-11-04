export CELL=c119 IP=192.0.3.194 Z=c119.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R8archprog-$CELL.log) 2>&1
bash $A/kitpush.sh c119 > /dev/null 2>&1
PR=$(domain_id progc.test); echo "== uploaded program on Arch via panel-runtime.sh (progc.test id=$PR) $(ts)"
as custc GET /api/v1/me/guide-state > /dev/null
cat $T/cred-custc.json.cookie | timeout 1500 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/kit/deploy/e2e/sandbox-escape/prt.sh $PR /nonexistent program" > $T/prt-progc.txt 2>&1; echo "rc=$?"
cut -c1-300 $T/prt-progc.txt; cp $T/prt-progc.txt $LOGS/panel-runtime-program-$CELL.txt
