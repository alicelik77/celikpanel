# D4 proof on Debian 12 postfwd 1.35 (lab only): labnet + one plain Debian 12 guest.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
mkdir -p $B $LOGS; bash $A/sync.sh
python3 -I $L/lab.py net-up > /dev/null; python3 -I $L/lab.py stop beta1-lab labnet > /dev/null 2>&1
timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up $? $(ts)"
b1 provision pf12 main debian12 192.0.3.230 frankfurt 2048 > $LOGS/prov-pf12-main.log 2>&1; tail -c 150 $LOGS/prov-pf12-main.log; echo
lab ssh pf12 main -- "sudo -n tee /root/g_pf135.sh >/dev/null" < $A/g_pf135.sh
echo "######## 128 drop-in (reload)"; lab ssh pf12 main -- "sudo -n bash /root/g_pf135.sh old" 2>&1 | tee $LOGS/D4-pf135-old.log
echo "######## new drop-in (restart + save_rates)"; lab ssh pf12 main -- "sudo -n bash /root/g_pf135.sh" 2>&1 | tee $LOGS/D4-pf135-new.log
