source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
lab ssh r9 backup -- "sudo -n bash -c \"systemctl list-units --failed --no-legend --plain | cat; echo (failed end); cat /opt/celikpanel/release.commit; systemctl is-active named celikpanel-panel\"" > $LOGS/final-r9-backup.txt 2>&1; cat $LOGS/final-r9-backup.txt
python3 -I $L/lab.py stop r9 backup | tr -d "\n "; echo; rm -rf /var/tmp/cp-beta1/lab/cells/r9/backup
free -m | sed -n 2p
