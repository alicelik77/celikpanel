source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/lab/kit/env.sh
pkill -f 'kit/singleA.sh' -P 1 2>/dev/null; pgrep -af 'singleA' | cut -c1-120
for c in u22 d12; do echo "== $c"; lab ssh $c main -- "sudo -n tee /root/g_sock.sh >/dev/null" < $A/g_sock.sh; lab ssh $c main -- "sudo -n bash /root/g_sock.sh"; done 2>&1 | tee $LOGS/socket-mode-u22-d12.txt
