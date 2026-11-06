source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit/env.sh
b1 sh p6 main "systemctl reset-failed lab-busy.service; systemctl list-units --failed --all --no-legend --plain | cat; echo end" --label rf | python3 -c 'import json,sys; print(json.load(sys.stdin)["stdout"])'
