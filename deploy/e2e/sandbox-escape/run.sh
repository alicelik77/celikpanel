#!/bin/bash
# Site sandbox escape-test kit (README.md). Runs each probe under a unit
# rendered exactly as CelikPanel renders one and reports PASS when every
# escape attempt of the matrix (items 1-18 in docs/SITE-SANDBOX.md) fails.
#
#   ./run.sh build            build bin/render, bin/goprobe and
#                             bin/sandbox-helper (needs Go; run on the
#                             development machine)
#   sudo ./run.sh             the matrix, on a lab VM (root, systemd, nft)
#   sudo ./run.sh reboot-prepare / reboot-verify
#                             item 17 across a reboot
#
# Lab VMs only: it creates a test account, two test sites under
# /var/www/celikpanel/subscriptions/99000{1,2}, units celikrt-99000{1,2,3}
# and a throwaway network namespace, and removes all of them at the end.
# It never runs "nft flush ruleset" and never touches inet celikpanel_fw.
#
# Yalniz deneme sanal makinelerinde calistirin.
set -u
KIT=$(cd "$(dirname "$0")" && pwd)
BIN=$KIT/bin
USER_NAME=cpsbxprobe
SUBS=/var/www/celikpanel/subscriptions
HOME_DIR=$SUBS/990001/sites/990001
APP=$HOME_DIR/app
OTHER=$SUBS/990002/sites/990002/secret.txt
UNIT_GO=celikrt-990001.service
UNIT_PY=celikrt-990002.service
UNIT_LOAD=celikrt-990003.service
PORT_GO=34990
PORT_PY=34991
PORT_LOAD=34992
NS=celiksbxns
EGRESS=/usr/lib/celikpanel-sandbox/egress.nft
INSTALLED_EGRESS=/run/celikpanel-sandbox-kit-installed-egress
HELPER=/usr/lib/celikpanel-sandbox/sandbox-helper
INSTALLED_HELPER=/run/celikpanel-sandbox-kit-installed-helper
BIND_TEMPLATE=/etc/systemd/system/celikpanel-sandbox@.service
INSTALLED_TEMPLATE=/run/celikpanel-sandbox-kit-installed-template
declare -A RESULT DETAIL

log() { printf '%s\n' "$*"; }
mark() { # item verdict detail: FAIL wins over SKIP wins over PASS
	local item=$1 verdict=$2 detail=${3:-}
	case "${RESULT[$item]:-}" in
		FAIL) ;;
		SKIP) [ "$verdict" = FAIL ] && RESULT[$item]=FAIL ;;
		*) RESULT[$item]=$verdict ;;
	esac
	[ -n "$detail" ] && DETAIL[$item]="${DETAIL[$item]:-}${DETAIL[$item]:+; }$detail"
}

build() {
	command -v go >/dev/null || { log "Go is needed to build the kit"; exit 1; }
	local repo; repo=$(cd "$KIT/../../.." && pwd)
	mkdir -p "$BIN"
	(cd "$repo" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BIN/render" ./deploy/e2e/sandbox-escape/render) || exit 1
	(cd "$repo" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BIN/goprobe" ./deploy/e2e/sandbox-escape/probes/goprobe) || exit 1
	# The sandbox helper is the Agent started under this name.
	(cd "$repo" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$BIN/sandbox-helper" ./cmd/agent) || exit 1
	log "built $BIN/render, $BIN/goprobe and $BIN/sandbox-helper (static)"
}

preflight() {
	[ "$(id -u)" = 0 ] || { log "run as root on a lab VM"; exit 1; }
	for tool in systemctl journalctl nft ip ss python3 nsenter; do
		command -v $tool >/dev/null || { log "missing: $tool"; exit 1; }
	done
	[ -x "$BIN/render" ] && [ -x "$BIN/goprobe" ] || { log "run ./run.sh build first (on a machine with Go)"; exit 1; }
	[ -x "$HELPER" ] || [ -x "$BIN/sandbox-helper" ] || { log "run ./run.sh build first: bin/sandbox-helper is missing"; exit 1; }
	"$BIN/render" -what caps | tee /tmp/celik-sbx-caps.json
	LEVEL=$(python3 -c 'import json;print(json.load(open("/tmp/celik-sbx-caps.json"))["assessment"]["level"])')
	EGRESS_OK=$(python3 -c 'import json;c=json.load(open("/tmp/celik-sbx-caps.json"))["caps"];print(1 if c["nft_socket_cgroupv2"] and c["cgroup_v2"] else 0)')
	log "sandbox level: $LEVEL (egress enforced: $EGRESS_OK)"
}

setup() {
	mkdir -p "$APP/bin" "$APP/src" "$APP/data" "$(dirname "$OTHER")"
	# A regular account (uid 1000 or above), as CelikPanel's site accounts:
	# the helper refuses a system account.
	id "$USER_NAME" >/dev/null 2>&1 || useradd --home-dir "$HOME_DIR" --no-create-home --shell /usr/sbin/nologin "$USER_NAME"
	echo "another site's secret" > "$OTHER"; chmod 0644 "$OTHER"
	install -m 0755 "$BIN/goprobe" "$APP/bin/goprobe"
	install -m 0644 "$KIT/probes/probe.py" "$APP/src/probe.py"
	chown -R "$USER_NAME:$USER_NAME" "$HOME_DIR"
	chmod 0750 "$HOME_DIR"
	if [ ! -e "$EGRESS" ]; then
		# Not a CelikPanel host (or no app ran yet): install the same files
		# CelikPanel writes, and remove them at the end.
		mkdir -p /usr/lib/celikpanel-sandbox
		"$BIN/render" -what egress > "$EGRESS"
		"$BIN/render" -what guard-script > /usr/lib/celikpanel-sandbox/egress-guard.sh
		"$BIN/render" -what guard-service > /etc/systemd/system/celikpanel-egress-guard.service
		"$BIN/render" -what guard-timer > /etc/systemd/system/celikpanel-egress-guard.timer
		systemctl daemon-reload
		[ "$EGRESS_OK" = 1 ] && systemctl enable --now celikpanel-egress-guard.timer
		touch "$INSTALLED_EGRESS"
	fi
	if [ ! -e "$HELPER" ]; then
		install -D -m 0755 "$BIN/sandbox-helper" "$HELPER"
		touch "$INSTALLED_HELPER"
	fi
	if [ ! -e "$BIND_TEMPLATE" ]; then
		"$BIN/render" -what bind-template > "$BIND_TEMPLATE"
		touch "$INSTALLED_TEMPLATE"
		systemctl daemon-reload
	fi
}

render_unit() { # unit kind port exec env memory tasks cpu
	"$BIN/render" -what unit -unit "$1" -kind "$2" -user "$USER_NAME" -appdir "$APP" -port "$3" \
		-exec "$4" -env "$5" -memory "${6:-256}" -tasks "${7:-64}" -cpu "${8:-50}" > "/etc/systemd/system/$1" || return 1
	systemctl daemon-reload
}

wait_line() { # unit pattern seconds
	local i
	for i in $(seq 1 "$3"); do
		journalctl -u "$1" -o cat --since "@$STARTED" --no-pager 2>/dev/null | grep -q "$2" && return 0
		sleep 1
	done
	return 1
}

counter() { nft list counter inet celikpanel_egress smtp_rejects 2>/dev/null | sed -n 's/.*packets \([0-9]*\).*/\1/p' | head -1; }

in_set() { nft list set inet celikpanel_egress smtp_blocked 2>/dev/null | grep -q "system.slice/$1"; }

collect() { # unit kind
	local unit=$1 kind=$2 line item name verdict rest prefix
	declare -A seen=()
	while read -r line; do
		read -r prefix item name verdict rest <<<"$line"
		[ "$prefix" = SANDBOX-PROBE ] || continue
		case "$item" in [0-9]*) ;; *) continue ;; esac
		seen[$item]=1
		case "$verdict" in
			ESCAPED) mark "$item" FAIL "$kind $name: $rest" ;;
			BLOCKED) mark "$item" PASS ;;
			SKIP) mark "$item" PASS "$kind $name skipped: $rest" ;;
		esac
	done < <(journalctl -u "$unit" -o cat --since "@$STARTED" --no-pager)
	for item in 1 2 3 4 5 6 7 8 9 10 12 13 14 15 16; do
		[ -n "${seen[$item]:-}" ] || mark "$item" FAIL "$kind: no result for item $item"
	done
}

run_matrix() { # unit kind port exec
	local unit=$1 kind=$2 port=$3 exec=$4 before after
	before=$(counter); before=${before:-0}
	STARTED=$(date +%s)
	render_unit "$unit" "$kind" "$port" "$exec" "PROBE_APPDIR=$APP,PROBE_PORT=$port,PROBE_KIND=$kind,PROBE_OTHER_SITE=$OTHER" || { mark 13 FAIL "$kind: render failed"; return; }
	systemctl start "$unit"
	if ! wait_line "$unit" "SANDBOX-PROBE DONE" 120; then
		mark 1 FAIL "$kind probe did not finish: $(systemctl show -p Result --value "$unit")"
		journalctl -u "$unit" -o cat --since "@$STARTED" --no-pager | tail -20
		return
	fi
	collect "$unit" "$kind"
	after=$(counter); after=${after:-0}
	if [ "$EGRESS_OK" = 1 ]; then
		# Six loopback attempts (v4 and v6, three ports) must have hit the rule.
		[ $((after - before)) -ge 6 ] && mark 9 PASS || mark 9 FAIL "$kind: the SMTP counter moved by $((after - before)), not >= 6"
		in_set "$unit" || mark 17 FAIL "$kind: $unit is not in the SMTP set"
	else
		mark 9 FAIL "$kind: this host cannot hold the SMTP block (reduced level)"
	fi
	# The listen proof CelikPanel uses: loopback only.
	ss -Htlnp | grep -E ":$port\b" | grep -vqE "127\.0\.0\.1:$port|\[::1\]:$port" && mark 10 FAIL "$kind listens beyond loopback"
}

item17() { # restart and nft table removal, with the Go probe
	local before after
	STARTED=$(date +%s)
	systemctl restart "$UNIT_GO"
	wait_line "$UNIT_GO" "SANDBOX-PROBE DONE" 120 || { mark 17 FAIL "no restart"; return; }
	journalctl -u "$UNIT_GO" -o cat --since "@$STARTED" --no-pager | grep -q "13 tmp-persistence ESCAPED" && mark 13 FAIL "/tmp survived a restart"
	[ "$EGRESS_OK" = 1 ] || { mark 17 FAIL "reduced level: no SMTP block"; return; }
	in_set "$UNIT_GO" && mark 17 PASS || mark 17 FAIL "not in the set after a restart"
	log "removing inet celikpanel_egress (what 'nft flush ruleset' does to it); the guard must restore it within 45 s"
	nft delete table inet celikpanel_egress
	local i
	for i in $(seq 1 45); do in_set "$UNIT_GO" && break; sleep 1; done
	in_set "$UNIT_GO" || { mark 17 FAIL "the guard did not restore the block within 45 s"; return; }
	before=$(counter); before=${before:-0}
	STARTED=$(date +%s)
	python3 -c "import urllib.request;urllib.request.urlopen('http://127.0.0.1:$PORT_GO/smtp',timeout=60).read()" || mark 17 FAIL "the probe did not answer /smtp"
	journalctl -u "$UNIT_GO" -o cat --since "@$STARTED" --no-pager | grep -q "^SANDBOX-PROBE 9 .* ESCAPED" && mark 17 FAIL "mail ports open after the table was removed"
	after=$(counter); after=${after:-0}
	[ $((after - before)) -ge 6 ] && mark 17 PASS || mark 17 FAIL "after the guard: counter moved by $((after - before))"
}

item18() { # the app folder swapped for a link to another site
	local pid
	if ! systemctl is-active -q "$UNIT_GO"; then
		STARTED=$(date +%s)
		render_unit "$UNIT_GO" program "$PORT_GO" "$APP/bin/goprobe" "PROBE_APPDIR=$APP,PROBE_PORT=$PORT_GO,PROBE_KIND=program,PROBE_OTHER_SITE=$OTHER"
		systemctl start "$UNIT_GO"
		wait_line "$UNIT_GO" "SANDBOX-PROBE DONE" 120 || { mark 18 FAIL "the Go probe did not start"; return; }
	fi
	# With PrivatePIDs (systemd 257+) the main PID is the "(sd-pidns-child)"
	# process outside the final mount namespace: take the program itself
	# from the unit's cgroup.
	pid=""
	for p in $(cat "/sys/fs/cgroup/system.slice/$UNIT_GO/cgroup.procs" 2>/dev/null); do
		case "$(cat "/proc/$p/comm" 2>/dev/null)" in "(sd-pidns"*) ;; *) pid=$p ;; esac
	done
	[ -n "$pid" ] || { mark 18 FAIL "the program's process was not found"; return; }
	mv "$APP" "$APP.moved" && ln -s "$(dirname "$OTHER")" "$APP" || { mark 18 FAIL "the folder could not be swapped"; return; }
	# The running program keeps its own folder: the mount holds the folder,
	# not its name.
	if nsenter -t "$pid" -m test -x "$APP/bin/goprobe" && ! nsenter -t "$pid" -m test -e "$APP/secret.txt"; then
		mark 18 PASS
	else
		mark 18 FAIL "after the swap the running program sees another folder"
	fi
	# A restart is refused by the helper, and nothing of the other site is
	# bound.
	STARTED=$(date +%s)
	systemctl restart "$UNIT_GO" 2>/dev/null
	sleep 2
	systemctl is-active -q "$UNIT_GO" && mark 18 FAIL "the unit started with a link in place of its folder"
	journalctl -u "celikpanel-sandbox@${UNIT_GO%.service}.service" -o cat --since "@$STARTED" --no-pager | grep -q "celikpanel-sandbox: $UNIT_GO was not started (app_folder_not_real)" \
		&& mark 18 PASS || mark 18 FAIL "no refusal of the helper in the bind unit's journal"
	findmnt -rn -o SOURCE,TARGET | grep -q "990002" && mark 18 FAIL "the other site is mounted somewhere"
	# The real folder back: the unit starts again.
	systemctl stop "$UNIT_GO" 2>/dev/null
	rm -f "$APP"; mv "$APP.moved" "$APP"
	systemctl reset-failed "$UNIT_GO" 2>/dev/null
	STARTED=$(date +%s)
	systemctl start "$UNIT_GO"
	wait_line "$UNIT_GO" "SANDBOX-PROBE DONE" 120 && mark 18 PASS || mark 18 FAIL "the unit did not start again with its real folder"
	systemctl stop "$UNIT_GO"
	[ -e "/run/celikpanel-sandbox/$UNIT_GO" ] && mark 18 FAIL "the stop left /run/celikpanel-sandbox/$UNIT_GO behind"
}

item10_bind_any() {
	STARTED=$(date +%s)
	render_unit "$UNIT_LOAD" program "$PORT_LOAD" "$APP/bin/goprobe" "PROBE_MODE=bind-any,PROBE_PORT=$PORT_LOAD" || return
	systemctl start "$UNIT_LOAD"
	if ! wait_line "$UNIT_LOAD" "bind-any" 30; then mark 10 FAIL "bind-any probe did not start"; return; fi
	if journalctl -u "$UNIT_LOAD" -o cat --since "@$STARTED" --no-pager | grep -q "10 bind-any BLOCKED"; then
		mark 10 PASS; systemctl stop "$UNIT_LOAD"; return
	fi
	# It listens on 0.0.0.0:port (SocketBind allows its own port on any
	# address): CelikPanel's listen proof must see it, and nothing outside
	# may reach it. A veth into a throwaway namespace is "outside".
	ss -Htlnp | grep -q "0.0.0.0:$PORT_LOAD" && log "listen proof: 0.0.0.0:$PORT_LOAD is seen (CelikPanel stops such an app)"
	ip netns add $NS; ip link add celiksbxh type veth peer name celiksbxp; ip link set celiksbxp netns $NS
	ip addr add 10.250.250.1/30 dev celiksbxh; ip link set celiksbxh up
	ip netns exec $NS ip addr add 10.250.250.2/30 dev celiksbxp; ip netns exec $NS ip link set celiksbxp up; ip netns exec $NS ip link set lo up
	python3 -c "import socket,time
s=socket.socket();s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind(('10.250.250.1',$((PORT_LOAD+1))));s.listen(5);time.sleep(20)" &
	local control=$!
	sleep 1
	local sandboxed=open reference=open
	ip netns exec $NS timeout 5 bash -c "</dev/tcp/10.250.250.1/$PORT_LOAD" 2>/dev/null || sandboxed=closed
	ip netns exec $NS timeout 5 bash -c "</dev/tcp/10.250.250.1/$((PORT_LOAD+1))" 2>/dev/null || reference=closed
	kill $control 2>/dev/null
	if [ "$sandboxed" = open ]; then mark 10 FAIL "a sandboxed 0.0.0.0 listener was reachable from outside"
	elif [ "$reference" = closed ]; then mark 10 SKIP "the host firewall drops both; inconclusive for the egress table's input rule"
	else mark 10 PASS; fi
	systemctl stop "$UNIT_LOAD"
}

item11_load() {
	local mode check
	for mode in fork memory cpu logflood; do
		STARTED=$(date +%s)
		render_unit "$UNIT_LOAD" program "$PORT_LOAD" "$APP/bin/goprobe" "PROBE_MODE=$mode" 256 64 50 || continue
		systemctl reset-failed "$UNIT_LOAD" 2>/dev/null
		systemctl start "$UNIT_LOAD"
		case $mode in
			fork)
				wait_line "$UNIT_LOAD" "fork-bomb" 60
				journalctl -u "$UNIT_LOAD" -o cat --since "@$STARTED" --no-pager | grep -q "fork-bomb BLOCKED" && mark 11 PASS || mark 11 FAIL "fork bomb not limited" ;;
			memory)
				for check in $(seq 1 60); do [ "$(systemctl show -p Result --value "$UNIT_LOAD")" = oom-kill ] && break; sleep 1; done
				[ "$(systemctl show -p Result --value "$UNIT_LOAD")" = oom-kill ] && mark 11 PASS || mark 11 FAIL "memory balloon not killed ($(systemctl show -p Result --value "$UNIT_LOAD"))" ;;
			cpu)
				wait_line "$UNIT_LOAD" "cpu done" 40
				local ns; ns=$(systemctl show -p CPUUsageNSec --value "$UNIT_LOAD")
				# 20 s of spinning at CPUQuota=50% is about 10 s of CPU.
				[ "${ns:-0}" -gt 0 ] && [ "$ns" -lt 14000000000 ] && mark 11 PASS || mark 11 FAIL "CPU used ${ns:-?} ns in 20 s at 50%" ;;
			logflood)
				wait_line "$UNIT_LOAD" "logflood done" 60; sleep 2
				local lines; lines=$(journalctl -u "$UNIT_LOAD" -o cat --since "@$STARTED" --no-pager | grep -c "SANDBOX-PROBE-FLOOD")
				[ "$lines" -lt 20000 ] && mark 11 PASS || mark 11 FAIL "$lines of 50000 flood lines reached the journal" ;;
		esac
		systemctl stop "$UNIT_LOAD"
	done
}

cleanup() {
	for u in $UNIT_GO $UNIT_PY $UNIT_LOAD; do
		systemctl stop "$u" 2>/dev/null; systemctl disable "$u" 2>/dev/null; rm -f "/etc/systemd/system/$u"
	done
	ip netns del $NS 2>/dev/null; ip link del celiksbxh 2>/dev/null
	[ -L "$APP" ] && { rm -f "$APP"; mv "$APP.moved" "$APP" 2>/dev/null; }
	if [ -e "$INSTALLED_HELPER" ]; then
		rm -f "$HELPER" "$INSTALLED_HELPER"
	fi
	if [ -e "$INSTALLED_TEMPLATE" ]; then
		rm -f "$BIND_TEMPLATE" "$INSTALLED_TEMPLATE"
	fi
	if [ -e "$INSTALLED_EGRESS" ]; then
		systemctl disable --now celikpanel-egress-guard.timer 2>/dev/null
		rm -f /etc/systemd/system/celikpanel-egress-guard.service /etc/systemd/system/celikpanel-egress-guard.timer
		rm -rf /usr/lib/celikpanel-sandbox
		nft delete table inet celikpanel_egress 2>/dev/null
		rm -f "$INSTALLED_EGRESS"
	fi
	systemctl daemon-reload
	userdel "$USER_NAME" 2>/dev/null
	rm -rf "$SUBS/990001" "$SUBS/990002"
	rmdir "$SUBS" /var/www/celikpanel 2>/dev/null; true
}

summary() {
	local item failed=0
	log ""; log "== site sandbox escape matrix (level: $LEVEL) =="
	for item in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18; do
		log "item $item: ${RESULT[$item]:-FAIL}${DETAIL[$item]:+ — ${DETAIL[$item]}}"
		[ "${RESULT[$item]:-FAIL}" = FAIL ] && failed=1
	done
	if [ $failed = 0 ]; then log "PASS"; else log "FAIL"; fi
	return $failed
}

case "${1:-matrix}" in
	build) build ;;
	matrix)
		preflight
		trap cleanup EXIT
		setup
		run_matrix "$UNIT_GO" program "$PORT_GO" "$APP/bin/goprobe"
		run_matrix "$UNIT_PY" python "$PORT_PY" "/usr/bin/python3 -I $APP/src/probe.py"
		item17
		item18
		systemctl stop "$UNIT_PY"
		item10_bind_any
		item11_load
		summary ;;
	quick)
		# The two probes only (no network namespace, no load, no table
		# removal): a smoke run on a development system.
		preflight
		trap cleanup EXIT
		setup
		run_matrix "$UNIT_GO" program "$PORT_GO" "$APP/bin/goprobe"
		run_matrix "$UNIT_PY" python "$PORT_PY" "/usr/bin/python3 -I $APP/src/probe.py"
		item18
		for item in 11 17; do RESULT[$item]=SKIP; DETAIL[$item]="not run in quick mode"; done
		summary ;;
	reboot-prepare)
		preflight; setup
		STARTED=$(date +%s)
		render_unit "$UNIT_GO" program "$PORT_GO" "$APP/bin/goprobe" "PROBE_APPDIR=$APP,PROBE_PORT=$PORT_GO,PROBE_KIND=program,PROBE_OTHER_SITE=$OTHER"
		printf '\n[Install]\nWantedBy=multi-user.target\n' >> "/etc/systemd/system/$UNIT_GO"
		systemctl daemon-reload; systemctl enable --now "$UNIT_GO"
		log "now reboot this VM, then run: sudo $0 reboot-verify" ;;
	reboot-verify)
		preflight
		trap cleanup EXIT
		STARTED=$(( $(date +%s) - 600 ))
		systemctl is-active -q "$UNIT_GO" || mark 17 FAIL "the unit did not start at boot"
		in_set "$UNIT_GO" && mark 17 PASS || mark 17 FAIL "not in the SMTP set after the reboot"
		before=$(counter); before=${before:-0}
		python3 -c "import urllib.request;urllib.request.urlopen('http://127.0.0.1:$PORT_GO/smtp',timeout=60).read()"
		after=$(counter); after=${after:-0}
		[ $((after - before)) -ge 6 ] && mark 17 PASS || mark 17 FAIL "after the reboot the counter moved by $((after - before))"
		LEVEL=reboot; for item in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 18; do RESULT[$item]=SKIP; done
		summary ;;
	*) log "usage: $0 [build|matrix|quick|reboot-prepare|reboot-verify]"; exit 2 ;;
esac
