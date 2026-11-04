#!/bin/bash
# guest (root): run the kit's panel-runtime.sh against this lab panel. Cookie on stdin (never on a command line).
# usage: prt.sh DOMAIN_ID OTHER_SITE_FILE KIND. Lab only.
read -r COOKIE
cd "$(dirname "$0")"
export LAB_ONLY=yes PANEL_URL=https://$(hostname -I | awk '{print $1}'):1923 COOKIE DOMAIN_ID=$1 OTHER_SITE_FILE=$2 KIND=$3 CURL_INSECURE=1 TIMEOUT=900
[ -f "$OTHER_SITE_FILE" ] || unset OTHER_SITE_FILE
# lab kit fix: the SMTP counter read must not end the script under pipefail when the egress table is not loaded yet
sed -i 's/head -1; }$/head -1 || true; }/' panel-runtime.sh; grep -n "^counter()" panel-runtime.sh
./panel-runtime.sh 2>&1
