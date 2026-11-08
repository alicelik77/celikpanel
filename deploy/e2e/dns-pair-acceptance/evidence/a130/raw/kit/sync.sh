W="/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/lab114/deploy/e2e/dns-pair-acceptance"
rm -rf /var/tmp/a130o/h; mkdir -p /var/tmp/a130o/h/deploy/e2e/dns-pair-acceptance
cd "$W" && tar --exclude=evidence --exclude=__pycache__ -cf - . | tar -xf - -C /var/tmp/a130o/h/deploy/e2e/dns-pair-acceptance
