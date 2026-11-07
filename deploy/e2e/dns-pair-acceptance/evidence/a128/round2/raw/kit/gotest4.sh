export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
cd /var/tmp/a128r1/esckit
export GOCACHE=/var/tmp/celikpanel-go-cache GOMODCACHE=/var/tmp/celikpanel-go-mod-cache GOFLAGS=-buildvcs=false
echo "== unmodified candidate 4ee915bb8 source, the three lane tests (archlinux host):"
timeout 900 go test -run 'TestAnEnabledVendorRepositoryShowsItsMissingStepsAndCompletesThemAsConfirmed|TestTheLaneNeverRewritesWhatTheOwnerWrote|TestAMaskedOrUnreadableUpdatesTimerBlocksTheLane' ./cmd/agent 2>&1 | grep -E "^(--- FAIL|ok|FAIL)"
