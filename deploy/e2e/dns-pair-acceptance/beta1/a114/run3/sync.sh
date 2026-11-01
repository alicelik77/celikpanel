# copy the lab branch harness from the worktree to the lab host
W="/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/lab114/deploy/e2e/dns-pair-acceptance"
mkdir -p /var/tmp/a114r3/h/deploy/e2e
rm -rf /var/tmp/a114r3/h/deploy/e2e/dns-pair-acceptance
mkdir -p /var/tmp/a114r3/h/deploy/e2e/dns-pair-acceptance
cd "$W" && tar --exclude=evidence -cf - . | tar -xf - -C /var/tmp/a114r3/h/deploy/e2e/dns-pair-acceptance
