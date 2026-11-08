A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
until grep -q "c10upd done" $A/c10upd.out 2>/dev/null; do sleep 15; done
bash $A/c3node2.sh > $A/c3node2.out 2>&1
echo chain5 done
