source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
lab ssh beta1-lab labnet -- "sudo -n tee /root/n_sw3.sh >/dev/null" < $A/n_sw3.sh
timeout 60 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_sw3.sh; which swaks"
