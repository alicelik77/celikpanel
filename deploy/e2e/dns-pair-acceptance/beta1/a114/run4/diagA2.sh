source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
bash $H/gq.sh a6-a backup "ps -eo pid,etimes,pcpu,args --sort=-pcpu | head -8 | cut -c1-200; journalctl -u celikpanel-agent --no-pager -o short-iso --since -8min | tail -12 | cut -c1-260; free -m | head -2"
