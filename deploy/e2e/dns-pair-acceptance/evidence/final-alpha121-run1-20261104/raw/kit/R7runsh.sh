# alpha.119 A item 3: the kit's run.sh full matrix (items 1-18 incl. 11, 17, 18) on the lab guest as root. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R7runsh-$CELL.log) 2>&1
echo "== run.sh matrix on $CELL $(ts)"
timeout 2400 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash -c 'cd /root/kit/deploy/e2e/sandbox-escape && ./run.sh 2>&1'" | grep -vE "^\s*$" | tail -80
echo "== run.sh done $(ts)"
