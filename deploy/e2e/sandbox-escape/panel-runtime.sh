#!/bin/bash
# Runs the kit's Go probe as a REAL runtime app (an uploaded program,
# docs/RUNTIME-APPS.md) through a lab panel's API, then judges the escape
# matrix from the panel's own log view, like run.sh does.
#
# LAB VM ONLY. It creates, deploys and (with REMOVE=1) removes one program on
# one domain of a lab panel. It never updates a panel, never touches the
# firewall and never runs anything on the server except, in UPLOAD=local
# mode, the upload of the probe as the site's own account.
#
#   ./run.sh build                       # once, on a machine with Go
#   LAB_ONLY=yes PANEL_URL=https://lab-vm:1923 COOKIE=<celikpanel_session> \
#     DOMAIN_ID=<id> [UPLOAD=local|sftp|none] ./panel-runtime.sh
#
# Environment:
#   LAB_ONLY=yes      required: you confirm the panel is a lab VM
#   PANEL_URL         https://host:port of the lab panel
#   COOKIE            the value of the celikpanel_session cookie of an account
#                     that owns the domain (the panel has no API tokens; log in
#                     with a browser and copy the cookie)
#   DOMAIN_ID         the lab domain (it needs a website and a certificate)
#   UPLOAD            local: run as root on the lab server, the probe is copied
#                     into <site home>/app/bin as the site's account (default
#                     when root); sftp: upload over SFTP (SFTP_TARGET); none:
#                     you uploaded bin/goprobe to app/bin/goprobe yourself
#   SFTP_TARGET       user@host of the domain's SFTP account (UPLOAD=sftp)
#   SFTP_PORT         default 22;  SFTP_IDENTITY: optional key file
#   OTHER_SITE_FILE   optional: a readable (0644) file of ANOTHER site on the
#                     same server, for matrix item 1
#   REMOVE=1          remove the program at the end; REMOVE_FILES=1 also
#                     removes its app folder
#   CURL_INSECURE=1   accept the lab panel's self-signed certificate
#   TIMEOUT           seconds to wait for an operation (default 600)
#
# Yalniz deneme sanal makinelerinde calistirin. Kurulu paneli asla guncellemez.
set -euo pipefail

KIT=$(cd "$(dirname "$0")" && pwd)
BIN=$KIT/bin
TIMEOUT=${TIMEOUT:-600}
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
BODY=$WORK/body
HTTP_CODE=000
FAILED=0
NOTES=()

log() { printf '%s\n' "$*"; }
die() { log "panel-runtime: $*"; exit 2; }
note() { NOTES+=("$*"); }
fail() { FAILED=1; NOTES+=("FAIL: $*"); }

# --- preflight -------------------------------------------------------------
[ "${LAB_ONLY:-}" = yes ] || die "set LAB_ONLY=yes to confirm that PANEL_URL is a lab VM (never a real server)"
[ -n "${PANEL_URL:-}" ] && [ -n "${COOKIE:-}" ] && [ -n "${DOMAIN_ID:-}" ] || die "PANEL_URL, COOKIE and DOMAIN_ID are required"
case "$PANEL_URL" in https://*) ;; *) die "PANEL_URL must start with https://" ;; esac
PANEL_URL=${PANEL_URL%/}
case "$DOMAIN_ID" in *[!0-9]*) die "DOMAIN_ID is a number" ;; esac
for tool in curl python3; do command -v $tool >/dev/null || die "missing: $tool"; done
[ -x "$BIN/goprobe" ] || die "run ./run.sh build first (on a machine with Go)"
if [ -z "${UPLOAD:-}" ]; then
	if [ "$(id -u)" = 0 ]; then UPLOAD=local; elif [ -n "${SFTP_TARGET:-}" ]; then UPLOAD=sftp; else UPLOAD=none; fi
fi
case "$UPLOAD" in local|sftp|none) ;; *) die "UPLOAD is local, sftp or none" ;; esac

# The session cookie goes through a private curl config file, never on the
# command line (other users could read it in the process list).
umask 077
printf 'header = "Cookie: celikpanel_session=%s"\nheader = "Origin: %s"\n' "$COOKIE" "$PANEL_URL" > "$WORK/curl.cfg"
CURL=(curl -sS --max-time 60 -K "$WORK/curl.cfg")
[ "${CURL_INSECURE:-}" = 1 ] && CURL+=(-k)
BASE="$PANEL_URL/api/v1/domains/$DOMAIN_ID/runtime-apps"

# api METHOD SUBPATH [JSON]: the body lands in $BODY and the status in
# $HTTP_CODE. Returns curl's exit code (0 when an answer arrived at all).
api() {
	local method=$1 path=$2 data=${3:-}
	local args=("${CURL[@]}" -o "$BODY" -w '%{http_code}' -X "$method")
	: > "$BODY"
	[ -n "$data" ] && args+=(-H 'Content-Type: application/json' --data "$data")
	HTTP_CODE=$("${args[@]}" "$BASE$path") || { HTTP_CODE=000; return 1; }
}

# js EXPR: evaluates a Python expression over the last answer (v).
js() { python3 -c 'import json,sys
v=json.load(open(sys.argv[1]))
r=eval(sys.argv[2])
print("" if r is None else (json.dumps(r) if isinstance(r,(dict,list,bool)) else r))' "$BODY" "$1"; }

refusal() { # prints the panel's refusal: code, reason, words
	python3 -c 'import json,sys
try:
    v=json.load(open(sys.argv[1]))
except Exception:
    print(open(sys.argv[1]).read()[:500]); sys.exit()
print("%s/%s: %s" % (v.get("code",""), v.get("reason",""), v.get("error","")))' "$BODY"
}

# --- 1. the program on the domain ------------------------------------------
api GET "" || die "the panel did not answer at $PANEL_URL"
[ "$HTTP_CODE" = 200 ] || die "reading the Programs view failed (HTTP $HTTP_CODE): $(refusal)"
log "domain: $(js 'v["domain"]'); sandbox level: $(js 'v["sandbox"].get("level","unknown")') gaps: $(js 'v["sandbox"]["gaps"]')"
OFFER=$(js 'next((k for k in v["kinds"] if k["kind"]=="program"), {})')
log "program kind: $OFFER"
if [ "$(js 'next((k["offered"] and k["permission_granted"] for k in v["kinds"] if k["kind"]=="program"), False)')" != true ]; then
	die "uploaded programs are not open for this account. Who acts: the administrator offers runtime:program on the Add-ons page (and the plan grants program_apps)"
fi
ARGS='["-serve","-port={port}"]'
if [ "$(js 'v["app"] is not None')" = true ]; then
	[ "$(js 'v["app"]["entry"]')" = goprobe ] || die "this domain already has another program; use a separate lab domain or remove it first"
	log "reusing the existing probe app"
	api PUT /settings "{\"entry\":\"goprobe\",\"args\":$ARGS,\"health_path\":\"/\"}" || die "settings: no answer"
	[ "$HTTP_CODE" = 200 ] || die "settings refused (HTTP $HTTP_CODE): $(refusal)"
else
	api POST /create "{\"kind\":\"program\",\"entry\":\"goprobe\",\"args\":$ARGS,\"health_path\":\"/\"}" || die "create: no answer; read the Programs tab before you retry"
	[ "$HTTP_CODE" = 201 ] || die "create refused (HTTP $HTTP_CODE): $(refusal)"
	log "created: $(cat "$BODY")"
fi
api GET "" || die "the panel did not answer"
APP_ID=$(js 'v["app"]["id"]'); PORT=$(js 'v["app"]["port"]'); FOLDER=$(js 'v["app"]["folder"]')
DOMAIN=$(js 'v["domain"]'); UNIT=$(js 'v["app"]["unit"]')
log "app $APP_ID: unit $UNIT, port $PORT, folder $FOLDER"

# --- 2. upload the probe ---------------------------------------------------
case "$UPLOAD" in
	local)
		[ "$(id -u)" = 0 ] || die "UPLOAD=local runs as root on the lab server"
		HOME_DIR=$(dirname "$FOLDER")
		# The site's account is the one whose home is the site home (as the
		# Agent resolves it).
		OWNER=$(getent passwd | awk -F: -v h="$HOME_DIR" '$6==h {print $1; exit}')
		[ -n "$OWNER" ] && [ "$OWNER" != root ] || die "no site account has $HOME_DIR as its home"
		[ ! -L "$FOLDER" ] || die "$FOLDER is a link; the panel would refuse it (app_folder_not_owned)"
		runuser -u "$OWNER" -- mkdir -p "$FOLDER/bin"
		STAGED=$(mktemp /tmp/celik-goprobe.XXXXXX)
		install -m 0644 "$BIN/goprobe" "$STAGED"
		runuser -u "$OWNER" -- install -m 0750 "$STAGED" "$FOLDER/bin/goprobe"
		rm -f "$STAGED"
		log "uploaded as $OWNER: $FOLDER/bin/goprobe" ;;
	sftp)
		[ -n "${SFTP_TARGET:-}" ] || die "UPLOAD=sftp needs SFTP_TARGET=user@host"
		command -v sftp >/dev/null || die "missing: sftp"
		SFTP=(sftp -b - -P "${SFTP_PORT:-22}")
		[ -n "${SFTP_IDENTITY:-}" ] && SFTP+=(-i "$SFTP_IDENTITY")
		# An SFTP session starts in the site home: the target is app/bin.
		printf -- '-mkdir app\n-mkdir app/bin\nput %s app/bin/goprobe\nchmod 750 app/bin/goprobe\n' "$BIN/goprobe" \
			| "${SFTP[@]}" "$SFTP_TARGET" || die "the SFTP upload failed"
		log "uploaded over SFTP to app/bin/goprobe" ;;
	none) log "UPLOAD=none: app/bin/goprobe must already be uploaded (file manager or SFTP), executable by its owner" ;;
esac

# --- 3. environment: a run marker, a secret for the log check --------------
RUN=$(python3 -c 'import secrets;print("r"+secrets.token_hex(6))')
SECRET=$(python3 -c 'import secrets;print(secrets.token_hex(12))')
ENV=$(RUN=$RUN SECRET=$SECRET OTHER=${OTHER_SITE_FILE:-} python3 -c 'import json,os
e=[{"key":"PROBE_RUN","value":os.environ["RUN"],"secret":False},
   {"key":"PROBE_SECRET","value":os.environ["SECRET"],"secret":True}]
if os.environ["OTHER"]:
    e.append({"key":"PROBE_OTHER_SITE","value":os.environ["OTHER"],"secret":False})
print(json.dumps({"entries":e}))')
api PUT /env "$ENV" || die "environment: no answer; the values may or may not be saved, so run again"
[ "$HTTP_CODE" = 200 ] || die "environment refused (HTTP $HTTP_CODE): $(refusal)"
[ -n "${OTHER_SITE_FILE:-}" ] || note "item 1: OTHER_SITE_FILE not set, so the probe reads the run.sh default path (a missing file is not proof)"

counter() { nft list counter inet celikpanel_egress smtp_rejects 2>/dev/null | sed -n 's/.*packets \([0-9]*\).*/\1/p' | head -1; }
COUNTER_BEFORE=""
if [ "$UPLOAD" = local ] && command -v nft >/dev/null; then COUNTER_BEFORE=$(counter); COUNTER_BEFORE=${COUNTER_BEFORE:-0}; fi

# --- 4. deploy: one operation id, never a second one -----------------------
OPID="probe-$(date +%s)-$RUN"
for attempt in 1 2 3; do
	if api POST /deploy "{\"operation_id\":\"$OPID\"}"; then break; fi
	# No answer: the same id again is the lost-answer guard (it starts nothing twice).
	log "deploy: no answer (attempt $attempt); asking again with the same operation id"
	sleep 5
done
case "$HTTP_CODE" in
	200|202) log "deploy accepted: operation $OPID" ;;
	000) die "deploy: the panel never answered; read the Programs tab (operation $OPID) before you retry" ;;
	*) die "deploy refused (HTTP $HTTP_CODE): $(refusal)" ;;
esac

# Polling only reads: GET without ?health=1. It never posts again.
wait_operation() { # operation id; sets OP_STATE OP_STEP OP_REASON OP_MESSAGE OP_CLASS
	local id=$1 deadline=$(( $(date +%s) + TIMEOUT )) last=""
	OP_STATE=unknown
	while [ "$(date +%s)" -lt "$deadline" ]; do
		if api GET "" && [ "$HTTP_CODE" = 200 ]; then
			if [ "$(js 'v["app"] is None')" = true ]; then OP_STATE=removed; return 0; fi
			if [ "$(js '(v["operation"] or {}).get("id")')" = "$id" ]; then
				OP_STATE=$(js 'v["operation"]["state"]'); OP_STEP=$(js 'v["operation"]["step"]')
				[ "$OP_STEP" != "$last" ] && log "  step: $OP_STEP ($OP_STATE)" && last=$OP_STEP
				if [ "$OP_STATE" != running ]; then
					OP_CLASS=$(js 'v["operation"].get("class","")'); OP_REASON=$(js 'v["operation"].get("reason","")')
					OP_MESSAGE=$(js 'v["operation"].get("message","")')
					return 0
				fi
			fi
		else
			log "  status read failed (HTTP $HTTP_CODE); the result is unknown, reading again"
		fi
		sleep 5
	done
	OP_STATE=timeout
}
wait_operation "$OPID"
case "$OP_STATE" in
	succeeded) log "deploy succeeded: https://$DOMAIN/ serves the probe" ;;
	failed) fail "deploy failed at $OP_STEP ($OP_CLASS/$OP_REASON): $OP_MESSAGE" ;;
	*) fail "deploy did not end within ${TIMEOUT}s (state $OP_STATE); it continues on the panel" ;;
esac

# --- 5. the probe's lines from the panel's log view ------------------------
# A deployed probe logged its last line before it listened; a failed deploy
# is read once.
TRIES=1; [ "$OP_STATE" = succeeded ] && TRIES=6
for i in $(seq 1 "$TRIES"); do
	api GET "/logs?lines=500" || true
	if [ "$HTTP_CODE" = 200 ] && python3 -c 'import json,sys
lines=json.load(open(sys.argv[1]))["lines"]
sys.exit(0 if any("SANDBOX-PROBE DONE" in l for l in lines) else 1)' "$BODY"; then break; fi
	sleep 5
done
[ "$HTTP_CODE" = 200 ] || fail "the log could not be read (HTTP $HTTP_CODE): $(refusal)"
cp "$BODY" "$WORK/logs.json"
VERDICT=$(python3 - "$WORK/logs.json" "$RUN" "$SECRET" "$PORT" <<'PY'
import json, sys
lines = json.load(open(sys.argv[1])).get("lines", [])
run, secret, port = sys.argv[2], sys.argv[3], sys.argv[4]
marks = [i for i, l in enumerate(lines) if "SANDBOX-PROBE-RUN " + run in l]
if not marks:
    print("FAIL: no line of this run in the log (the probe never started, or the log was cut)")
    sys.exit()
mine = lines[marks[-1]:]
result, detail = {}, {}
def mark(item, verdict, note=""):
    cur = result.get(item)
    if cur == "FAIL" or (cur == "SKIP" and verdict == "PASS"):
        pass
    else:
        result[item] = verdict
    if note:
        detail.setdefault(item, []).append(note)
done = False
for l in mine:
    if secret in l:
        mark("log", "FAIL", "a secret value reached the log view unredacted")
    at = l.find("SANDBOX-PROBE ")
    if at < 0:
        continue
    words = l[at:].split(" ", 4)
    if len(words) >= 2 and words[1] == "DONE":
        done = True
        continue
    if len(words) < 4 or not words[1].isdigit():
        continue
    item, name, verdict = words[1], words[2], words[3]
    rest = words[4] if len(words) > 4 else ""
    if verdict == "ESCAPED":
        mark(item, "FAIL", name + ": " + rest)
    elif verdict == "BLOCKED":
        mark(item, "PASS")
    elif verdict == "SKIP":
        mark(item, "PASS", name + " skipped: " + rest)
if not done:
    mark("1", "FAIL", "the probe did not finish (no SANDBOX-PROBE DONE)")
for item in ["1","2","3","4","5","6","7","8","9","10","12","13","14","15","16"]:
    if item not in result:
        mark(item, "FAIL", "no result for item " + item)
if not any("SANDBOX-PROBE-ENVECHO" in l for l in mine):
    mark("log", "FAIL", "the echo line of the secret is missing, so the redaction was not exercised")
elif "log" not in result:
    mark("log", "PASS")
env = next((l for l in mine if "SANDBOX-PROBE-ENV " in l), "")
if "host=127.0.0.1" in env and ("env_port=" + port) in env:
    mark("env", "PASS")
else:
    mark("env", "FAIL", "PORT/HOST not as expected: " + env[env.find("SANDBOX-PROBE-ENV"):])
for item in ["1","2","3","4","5","6","7","8","9","10","12","13","14","15","16","env","log"]:
    v = result.get(item, "FAIL")
    print("item %s: %s%s" % (item, v, (" - " + "; ".join(detail[item])) if item in detail else ""))
PY
)
log ""; log "== runtime app escape matrix (app $APP_ID on $DOMAIN) =="
log "$VERDICT"
grep -q ': FAIL' <<<"$VERDICT" && FAILED=1
grep -q '^FAIL' <<<"$VERDICT" && FAILED=1
if [ -n "$COUNTER_BEFORE" ]; then
	AFTER=$(counter); AFTER=${AFTER:-0}
	if [ $((AFTER - COUNTER_BEFORE)) -ge 6 ]; then log "item 9 counter: +$((AFTER - COUNTER_BEFORE)) rejects"
	else fail "item 9: the SMTP counter moved by $((AFTER - COUNTER_BEFORE)), not >= 6"; fi
else
	note "item 9: judged on the probe's lines only (the SMTP counter is read only with UPLOAD=local on the server)"
fi
note "items 11 and 17 (load limits, reboot, table removal) are run.sh's; not run here"

# --- 6. optional removal ---------------------------------------------------
if [ "${REMOVE:-}" = 1 ]; then
	RMID="probe-rm-$(date +%s)-$RUN"
	FILES=false; [ "${REMOVE_FILES:-}" = 1 ] && FILES=true
	for attempt in 1 2 3; do
		if api POST /remove "{\"operation_id\":\"$RMID\",\"remove_files\":$FILES,\"confirm_domain\":\"$DOMAIN\"}"; then break; fi
		sleep 5
	done
	case "$HTTP_CODE" in
		200|202)
			wait_operation "$RMID"
			if [ "$OP_STATE" = removed ]; then log "removed (files removed: $FILES)"
			else fail "removal did not finish: ${OP_STATE} ${OP_REASON:-} ${OP_MESSAGE:-}"; fi ;;
		*) fail "removal refused (HTTP $HTTP_CODE): $(refusal)" ;;
	esac
fi

for n in "${NOTES[@]}"; do log "$n"; done
if [ "$FAILED" = 0 ]; then log "PASS"; else log "FAIL"; fi
exit "$FAILED"
