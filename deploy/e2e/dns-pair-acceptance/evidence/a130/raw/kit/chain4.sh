A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
until grep -q "c2notify done" $A/c2notify.out 2>/dev/null; do sleep 15; done
bash $A/c10upd.sh > $A/c10upd.out 2>&1
bash $A/c4mail3.sh > $A/c4mail3.out 2>&1
echo chain4 done
