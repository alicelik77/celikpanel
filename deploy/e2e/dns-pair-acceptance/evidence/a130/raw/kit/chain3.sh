A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
bash $A/c9upd.sh > $A/c9upd.out 2>&1
for c in c3x c4mail2; do bash $A/$c.sh > $A/$c.out 2>&1; done
bash $A/c5db.sh main site1.id mysql > $A/c5db-main2.out 2>&1
for c in c3node c2notify; do bash $A/$c.sh > $A/$c.out 2>&1; done
echo chain3 done | tee $A/chain3.flag
