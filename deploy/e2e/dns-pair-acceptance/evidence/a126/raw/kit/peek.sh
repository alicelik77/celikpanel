source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/env.sh
date -u +%T; for n in main backup; do opnow p6 $n | head -2 | cut -c1-300; done; free -m | sed -n 2p; dmesg | grep -c "sub channel open failed"
