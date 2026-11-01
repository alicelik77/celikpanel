source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
lab ssh a4-c main -- "sudo -n sh -s" <<'R'
journalctl --no-pager -o cat -u celikpanel-self-update-f8d7f906999f78cde7d8cb3a675ea1f1.service | grep -vE '^\s*$' | tail -25 | cut -c1-300
echo ---- recovery status
/usr/libexec/celikpanel/recovery status --request-id f8d7f906999f78cde7d8cb3a675ea1f1 --json 2>&1 | head -c 1800; echo
R
