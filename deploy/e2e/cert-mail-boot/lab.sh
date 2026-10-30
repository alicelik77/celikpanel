#!/bin/bash
# Real-boot evidence for D-040 on Ubuntu 24.04 and Arch: a website
# certificate issued by the Agent's own code is renewed by the operating
# system's certbot timer on a booted system, still enabled and served after a
# reboot, by
# nginx, Postfix and Dovecot (Dovecot 2.3 on Ubuntu, 2.4 with CelikPanel's
# Arch main file on Arch) with no CelikPanel program present.
#
# Runs as root in the development lab (the `archlinux` WSL guest with
# QEMU/KVM). Each guest boots from the distribution's own cloud image in a
# fresh overlay: first with user-mode networking to install the release's
# packages from its mirrors, then, from the next boot on, with
# `restrict=on` (no route out of the guest), so nothing but the local Pebble
# test CA can be reached. The base images are read only and kept. Never run
# against an installed server.
#
# usage: lab.sh SOURCE_DIR OUT_DIR ubuntu-24.04|arch ...
# env:   IMAGES (default /var/tmp/cp-v3n28/images), WORK (default /var/tmp/cp-certmail-boot)
set -euo pipefail
src=${1:?source dir}; out=${2:?evidence dir}; shift 2
images=${IMAGES:-/var/tmp/cp-v3n28/images}
work=${WORK:-/var/tmp/cp-certmail-boot}
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
case "$work" in /var/tmp/cp-certmail-boot*) ;; *) echo "refusing work dir $work" >&2; exit 2 ;; esac
mkdir -p "$work" "$out"
log() { printf '== %s %s\n' "$(date -u +%H:%M:%S)" "$*" | tee -a "$out/lab.log"; }

export GOTOOLCHAIN=local GOFLAGS=-buildvcs=false CGO_ENABLED=0 GOOS=linux GOARCH=amd64
export GOCACHE=${GOCACHE:-/var/tmp/archparity-gocache-arch} GOMODCACHE=${GOMODCACHE:-/var/tmp/archparity-gomod-arch}
if [[ ! -x "$work/agent.test" ]]; then
  log "build the Agent's realsystem test binary"
  (cd "$src" && go test -c -tags realsystem -o "$work/agent.test" ./cmd/agent)
fi
if [[ ! -x "$work/pebble" ]]; then
  log "build Pebble v2.6.0 (the test ACME server; module proxy only)"
  GOBIN="$work" go install github.com/letsencrypt/pebble/v2/cmd/pebble@v2.6.0
fi
sha256sum "$work/agent.test" "$work/pebble" | tee -a "$out/lab.log"
[[ -f "$work/id" ]] || ssh-keygen -q -t ed25519 -N '' -f "$work/id"

declare -A image=( [ubuntu-24.04]=ubuntu-24.04-server-cloudimg-amd64-20260826.img
                   [arch]=Arch-Linux-x86_64-cloudimg-20260815.573966.qcow2 )
declare -A port=( [ubuntu-24.04]=22411 [arch]=22412 )

ssh_opts() { echo "-i $work/id -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=5 -p ${port[$1]}"; }
gssh() { local os=$1; shift; ssh $(ssh_opts "$os") lab@127.0.0.1 "$@"; }
gscp() { local os=$1; shift; scp $(ssh_opts "$os" | sed 's/-p /-P /') "$@"; }

boot() { # os label restrict(yes|no)
  local os=$1 label=$2 restrict=$3 nd=$work/$1 net
  net="user,id=n0,hostfwd=tcp:127.0.0.1:${port[$os]}-:22"
  [[ "$restrict" == yes ]] && net+=",restrict=on"
  qemu-system-x86_64 -name "cm-$os" -accel kvm -cpu host -smp 2 -m 2048 \
    -drive "file=$nd/overlay.qcow2,if=virtio,format=qcow2" \
    -drive "file=$nd/seed.iso,if=virtio,format=raw,media=cdrom,readonly=on" \
    -netdev "$net" -device virtio-net-pci,netdev=n0,mac=52:54:00:12:34:56 \
    -pidfile "$nd/qemu.pid" -serial "file:$nd/serial-$label.log" -display none -monitor none -daemonize
  local i
  for i in $(seq 1 120); do gssh "$os" true 2>/dev/null && { log "$os: boot $label up (restrict=$restrict)"; return 0; }; sleep 5; done
  log "$os: boot $label did not answer ssh"; return 1
}
halt() { # os
  local nd=$work/$1 pid i
  pid=$(cat "$nd/qemu.pid" 2>/dev/null || true)
  gssh "$1" sudo systemctl poweroff >/dev/null 2>&1 || true
  for i in $(seq 1 60); do [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null || return 0; sleep 2; done
  kill "$pid" 2>/dev/null || true
}

run_os() {
  local os=$1 nd=$work/$1
  rm -rf -- "$nd"
  mkdir -p "$nd/seed"
  local bootcmd="" runcmd=""
  # Arch's cloud image starts no sshd by itself, and its host keys appear
  # only in cloud-init's final stage (as in the beta1 lab).
  if [[ "$os" == arch ]]; then
    bootcmd=$'bootcmd:\n  - [systemctl, stop, --no-block, systemd-time-wait-sync.service]\n  - [systemctl, enable, sshd.service]\n'
    runcmd=$'runcmd:\n  - [systemctl, restart, sshd.service]\n'
  fi
  cat >"$nd/seed/user-data" <<EOF
#cloud-config
hostname: cm-$os
users:
  - name: lab
    sudo: ["ALL=(ALL) NOPASSWD:ALL"]
    shell: /bin/bash
    lock_passwd: true
    ssh_authorized_keys:
      - $(cat "$work/id.pub")
ssh_pwauth: false
package_update: false
${bootcmd}write_files:
  - path: /etc/celikpanel-cert-mail-boot-lab
    permissions: '0444'
    content: "disposable cert-mail boot lab guest; never an installed server\n"
  - path: /etc/celikpanel-sitecert-chroot
    permissions: '0444'
    content: "disposable lab guest (the realsystem test's marker)\n"
${runcmd}
EOF
  printf 'instance-id: cm-%s-%s\nlocal-hostname: cm-%s\n' "$os" "$(date +%s)" "$os" >"$nd/seed/meta-data"
  # Matched by the MAC boot() gives the NIC (Arch's renderer ignored a name glob).
  printf 'version: 2\nethernets:\n  nic:\n    match: {macaddress: "52:54:00:12:34:56"}\n    set-name: lab0\n    dhcp4: true\n' >"$nd/seed/network-config"
  genisoimage -quiet -output "$nd/seed.iso" -volid cidata -joliet -rock "$nd/seed/user-data" "$nd/seed/meta-data" "$nd/seed/network-config"
  qemu-img create -q -f qcow2 -F qcow2 -b "$images/${image[$os]}" "$nd/overlay.qcow2" 16G
  log "$os: base image ${image[$os]} sha256 $(sha256sum "$images/${image[$os]}" | cut -c1-16)…"

  boot "$os" install no
  log "$os: install the release's packages from its own mirrors"
  if [[ "$os" == arch ]]; then
    gssh "$os" 'sudo cloud-init status --wait >/dev/null 2>&1; sudo pacman -Syu --noconfirm --needed certbot nginx postfix dovecot openssl curl >/tmp/pkg.log 2>&1 || { tail -20 /tmp/pkg.log; exit 1; }'
  else
    gssh "$os" 'sudo cloud-init status --wait >/dev/null 2>&1; printf "postfix postfix/main_mailer_type select Internet Site\npostfix postfix/mailname string mx.site.test\n" | sudo debconf-set-selections; sudo DEBIAN_FRONTEND=noninteractive apt-get update -qq && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq certbot nginx postfix dovecot-imapd openssl curl >/tmp/pkg.log 2>&1 || { tail -20 /tmp/pkg.log; exit 1; }'
  fi
  halt "$os"

  # From here on the guest has no route out: only the local Pebble CA.
  boot "$os" first yes
  gssh "$os" 'curl -sS -m 5 -o /dev/null https://acme-v02.api.letsencrypt.org/directory && echo REACHABLE || echo "no route out (expected)"' | tee -a "$out/lab.log"
  gscp "$os" "$work/agent.test" "$work/pebble" "$here/guest.sh" lab@127.0.0.1:/tmp/
  gssh "$os" 'sudo install -m 0700 /tmp/agent.test /root/agent.test && sudo install -m 0755 /tmp/pebble /usr/local/bin/pebble && sudo install -m 0755 /tmp/guest.sh /root/guest.sh && rm -f /tmp/agent.test /tmp/pebble'
  local rc=0
  gssh "$os" 'sudo sh /root/guest.sh first' || rc=$?
  [[ $rc -eq 0 ]] && { gssh "$os" 'sudo sh /root/guest.sh renew' || rc=$?; }
  if [[ $rc -eq 0 ]]; then
    halt "$os"
    boot "$os" second yes
    gssh "$os" 'sudo sh /root/guest.sh second' || rc=$?
  fi
  if [[ $rc -eq 0 ]]; then
    gscp "$os" "$work/agent.test" lab@127.0.0.1:/tmp/
    gssh "$os" 'sudo install -m 0700 /tmp/agent.test /root/agent.test && rm -f /tmp/agent.test && sudo sh /root/guest.sh observe' || rc=$?
  fi
  mkdir -p "$out/$os"
  gssh "$os" 'sudo tar -C /root -czf - evidence' | tar -C "$out/$os" -xzf - || true
  cp "$nd"/serial-*.log "$out/$os/" 2>/dev/null || true
  halt "$os"
  log "$os: exit $rc"
  return $rc
}

status=0
for os in "$@"; do run_os "$os" || status=1; done
exit $status
