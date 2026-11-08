# Go tests of fix/a130-lab-b in the S2 guest (copy of the worktree). Lab only.
export PATH=/opt/celikpanel-s2/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin GOCACHE=/root/.cache/go-build GOFLAGS=-mod=mod
D=/var/tmp/a130-labb-src
rm -rf $D; mkdir -p $D
cd "/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/a130-lab-b" && tar --exclude=./web/node_modules --exclude=./.git -cf - . | tar -xf - -C $D
cd $D && gofmt -l cmd/agent | head; go vet ./cmd/agent 2>&1 | tail -3; go test ./cmd/agent -run "${1:-SSH|TurnOff|LockOut|TurnOn|Password}" -count=1 2>&1 | tail -15
