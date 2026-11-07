source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
E="/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/lab114/deploy/e2e/dns-pair-acceptance/evidence/a128-other-systems/raw"
mkdir -p "$E/kit"
cp $LOGS/* "$E/"
for v in 127 128 129 130; do cp $B/portal-v0.1.0-alpha.$v/site/releases/v0.1.0-alpha.$v/linux/amd64/release-manifest-v2 "$E/release-manifest-v2-$v" 2>/dev/null; cp $B/portal-v0.1.0-alpha.$v/get.sh.diff "$E/get.sh.diff-$v" 2>/dev/null; done
for f in out-build127.out out-build128.out out-build129.out out-build130.out; do cp $B/$f "$E/${f%.out}.txt"; done
cp $A/../u22upd-diag.txt $A/../pfd-d12.txt "$E/"
cd $A && cp *.sh *.py "$E/kit/" 2>/dev/null; rm -f "$E/kit/"look*.sh
grep -rlE "BEGIN (OPENSSH|PRIVATE)|\"password\": \"[^\[]" "$E" | head; du -sh "$E"; ls "$E" | wc -l
