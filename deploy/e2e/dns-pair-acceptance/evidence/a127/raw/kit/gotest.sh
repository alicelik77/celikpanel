export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
cd "/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/rel127"
export GOCACHE=/var/tmp/celikpanel-go-cache GOMODCACHE=/var/tmp/celikpanel-go-mod-cache GOFLAGS=-buildvcs=false
gofmt -l cmd/agent/backup_db.go cmd/agent/backup_pg_restore_prelude_test.go cmd/agent/backup_v2_linux_test.go cmd/agent/backup_pg_cluster_test.go cmd/agent/pg_statement_target_test.go
go vet ./cmd/agent 2>&1 | tail -5
timeout 1500 go test ./cmd/agent 2>&1 | tail -15
