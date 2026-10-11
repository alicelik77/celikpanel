#!/bin/bash
# run one command line in WSL archlinux as root, retrying while the WSL service does not answer (Wsl/Service/0x8007274c)
for i in $(seq 1 30); do
  out=$(MSYS_NO_PATHCONV=1 timeout 300 wsl -d archlinux -u root -- bash -c "$1" 2>&1); rc=$?
  if [ $rc -eq 127 ] && echo "$out" | tr -d '\0' | grep -q "0x8007274c\|W s l / S e r v i c e"; then sleep 15; continue; fi
  echo "$out"; exit $rc
done
echo "WSL did not answer after 30 tries"; exit 99
