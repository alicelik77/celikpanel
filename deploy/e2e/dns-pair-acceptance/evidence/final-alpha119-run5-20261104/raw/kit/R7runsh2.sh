source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R7runsh2-$CELL.log) 2>&1
echo "== kit fix $(ts)"; gq g_kitfix.sh
echo "== run.sh matrix (fixed kit) on $CELL $(ts)"
timeout 2400 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash -c 'cd /root/kit/deploy/e2e/sandbox-escape && ./run.sh 2>&1; echo; echo \"## mounts with 99000x after the run\"; findmnt -rn -o SOURCE,TARGET | grep 99000 | head; ls /run/celikpanel-sandbox/ 2>&1'" | grep -vE "^\s*$" | tail -60
echo "== run.sh done $(ts)"
