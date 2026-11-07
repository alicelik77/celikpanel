# labnet (root): one SMTP conversation. usage: n_swaks.sh SERVER PORT FROM TO [AUTHUSER] (password on stdin when AUTHUSER).
# Prints only the final reply lines (no secrets). Lab only.
S=$1; P=$2; F=$3; TO=$4; U=${5:-}
args=(--server "$S" --port "$P" --from "$F" --to "$TO" --h-Subject "a128 lab $(date +%s%N)" --body "a128 lab message" --timeout 30 --hide-informational --suppress-data)
if [ -n "$U" ]; then IFS= read -r PW; args+=(-tls --auth PLAIN --auth-user "$U" --auth-password "$PW" --hide-send); fi
[ "$P" = 465 ] && args+=(--tlsc)
out=$(swaks "${args[@]}" 2>&1)
echo "$out" | grep -vE "334|AUTH|^ *->|^~>|221 |Connection closed|^=== " | grep -E "^<|\*\*\*" | grep -vE "250[- ](CHUNKING|PIPELINING|SIZE|VRFY|ETRN|STARTTLS|AUTH|ENHANCED|8BITMIME|DSN|SMTPUTF8)|^<.{0,3}220|2\.1\.[05] Ok|354 " | tail -2 | cut -c1-230 | sed 's/^/  /'
