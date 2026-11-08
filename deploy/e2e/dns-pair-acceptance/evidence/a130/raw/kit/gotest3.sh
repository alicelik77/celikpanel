export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
grep -A8 -E "^--- FAIL" /tmp/a127-agent-test.txt | head -60
cd "/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/rel127"
export GOCACHE=/var/tmp/celikpanel-go-cache GOMODCACHE=/var/tmp/celikpanel-go-mod-cache GOFLAGS=-buildvcs=false
git stash list | head -2 >/dev/null
