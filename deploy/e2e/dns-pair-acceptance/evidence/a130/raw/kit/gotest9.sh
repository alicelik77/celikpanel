export PATH=/opt/celikpanel-s2/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
S=/var/tmp/a129-lab2-src; mkdir -p $S
cd "/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/a129-lab2"
tar --exclude=./web/node_modules --exclude=./.git -cf - go.mod go.sum cmd internal | (cd $S && tar -xf -)
cd $S; export GOCACHE=/var/tmp/celikpanel-go-cache GOMODCACHE=/var/tmp/celikpanel-go-mod-cache GOFLAGS=-buildvcs=false
gofmt -l cmd/panel/database_remote_test.go cmd/panel/database_remote_handlers.go
timeout 1200 go test ./cmd/panel -run 'DatabaseRemote' 2>&1 | tail -5
