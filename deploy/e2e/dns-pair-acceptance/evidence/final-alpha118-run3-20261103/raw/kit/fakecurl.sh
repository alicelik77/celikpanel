#!/bin/bash
# Lab stand-in for the network path to https://celikpanel.net (the test seam of
# deploy/test-fresh-install-entry-boundary-contract.sh): a celikpanel.net URL is
# answered from the lab portal copy in /root/lab-portal/site; any other URL goes
# to the real curl unchanged. Only the owner session's PATH has this. Lab only.
site=/root/lab-portal/site
log=/root/lab-portal/requests.log
args=("$@")
url= out= wo= max=
while (($#)); do
  case "$1" in
    -o|--output) out=$2; shift 2 ;;
    -w|--write-out) wo=$2; shift 2 ;;
    --max-filesize) max=$2; shift 2 ;;
    --proto|--connect-timeout|--retry) shift 2 ;;
    https://celikpanel.net/*) url=$1; shift ;;
    *) shift ;;
  esac
done
[ -n "$url" ] || exec /usr/bin/curl "${args[@]}"
rel=${url#https://celikpanel.net/}
src=$site/$rel
printf '%s %s\n' "$(date -u +%FT%TZ)" "$url" >> $log
if [ ! -f "$src" ] || [ -L "$src" ]; then
  echo "curl: (22) The requested URL returned error: 404" >&2; exit 22
fi
if [ -n "$max" ] && [ "$(stat -Lc %s "$src")" -gt "$max" ]; then
  echo "curl: (63) Maximum file size exceeded" >&2; exit 63
fi
if [ -n "$out" ]; then cp -- "$src" "$out"; else cat -- "$src"; fi
[ -z "$wo" ] || printf '200'
