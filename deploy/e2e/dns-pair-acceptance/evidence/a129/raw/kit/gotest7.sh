export PATH=/opt/celikpanel-s2/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
S=/var/tmp/a129-lab2-src; mkdir -p $S
cd "/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/a129-lab2"
tar --exclude=./web/node_modules --exclude=./.git -cf - go.mod go.sum cmd internal | (cd $S && tar -xf -)
cd $S; export GOCACHE=/var/tmp/celikpanel-go-cache GOMODCACHE=/var/tmp/celikpanel-go-mod-cache GOFLAGS=-buildvcs=false
gofmt -l internal/dbremote cmd/agent/database_remote*.go
go vet ./internal/dbremote 2>&1 | tail -3
go test ./internal/dbremote 2>&1 | tail -3
go test ./cmd/agent -run 'MariaDB|DatabaseRemote|PGRemote' -v 2>&1 | grep -E '^(--- FAIL|FAIL|ok|panic)|HealsTheCert|ReplacesTheEarlier|CertRoot' | head -20
