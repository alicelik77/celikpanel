A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
until grep -qE "== pair done" $A/pairP.out; do sleep 15; done
sleep 30
bash $A/c1pre.sh > $A/c1pre.out 2>&1
bash $A/c1upd.sh > $A/c1upd.out 2>&1
echo chain1 done
