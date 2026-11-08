V=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/view; mkdir -p $V
cp /var/tmp/a130o/shots/$1* $V/; ls $V | grep "^$1" | wc -l
