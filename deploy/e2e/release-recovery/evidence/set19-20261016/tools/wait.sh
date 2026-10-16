#!/bin/bash
# set19: wait (bounded, read-only) until a hand directory's file or a cell's rc file exists. usage: wait.sh HANDFILE RCFILE LIMIT_S
hand=$1; rc=$2; limit=${3:-1800}; t=0
until [ -e "$hand" ] || [ -e "$rc" ] || [ $t -ge $limit ]; do sleep 10; t=$((t+10)); done
date -u +%FT%TZ; echo "waited ${t}s"; ls -la "$(dirname "$hand")" 2>&1 | tail -n 8; cat "$rc" 2>/dev/null
