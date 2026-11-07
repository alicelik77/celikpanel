date
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/env.sh
timeout 30 python3 -I $L/lab.py ssh r8 main -- "sudo -n sh -c 'ps -eo etime,args --sort=-etime | grep -vE \"\[|sshd|systemd|agetty|dbus|cron\" | tail -12 | cut -c1-160; tail -5 /root/*.log 2>/dev/null | cut -c1-200'"
