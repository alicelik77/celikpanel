export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
cd "/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/rel127"
export GOCACHE=/var/tmp/celikpanel-go-cache GOMODCACHE=/var/tmp/celikpanel-go-mod-cache GOFLAGS=-buildvcs=false
timeout 1500 go test ./cmd/agent > /tmp/a127-agent-test.txt 2>&1
grep -E "^(--- FAIL|FAIL|ok|panic)" /tmp/a127-agent-test.txt | head -30
