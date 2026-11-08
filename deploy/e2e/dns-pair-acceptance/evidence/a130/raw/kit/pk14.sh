pkill -f p129c.sh; pkill -f k4remote.sh; sleep 1; pgrep -af "p129c|k4remote" | grep -v pgrep | wc -l
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
mv $LOGS/C4remote-r9-mysql.log $LOGS/C4remote-r9-mysql-run1-nofail2ban.log; tail -5 $LOGS/C4remote-r9-mysql-run1-nofail2ban.log | cut -c1-200
