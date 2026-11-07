# bg.sh SCRIPT OUT: run a kit script detached from the wsl session
setsid nohup bash /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/$1 > $2 2>&1 < /dev/null &
disown; echo started $1
