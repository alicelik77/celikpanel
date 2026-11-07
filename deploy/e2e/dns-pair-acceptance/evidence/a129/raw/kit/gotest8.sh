export PATH=/opt/celikpanel-s2/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
cd /var/tmp/a129-lab2-src; export GOCACHE=/var/tmp/celikpanel-go-cache GOMODCACHE=/var/tmp/celikpanel-go-mod-cache GOFLAGS=-buildvcs=false
go test ./cmd/agent -run 'HealsTheCert' -v 2>&1 | head -20
