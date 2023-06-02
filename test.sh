#!/usr/bin/env bash
set -e

: "${INDEX:=1}"

if [[ $(id -u) != 0 ]]; then
  exec sudo "$0" "$@"
fi

modprobe nbd
umount /tmp/nbd/mnt &>/dev/null || true
rm -rf /tmp/nbd/
mkdir -p /tmp/nbd/root

mke2fs -N 0 -O '^64bit' -d /tmp/nbd/root -m 5 -r 1 -t ext4 /tmp/nbd/image.ext4 '100M' &>/dev/null

/usr/local/go/bin/go build -o . ./...

# Clean up from previous run
./nbd disc -index=0 &>/dev/null

NLDEBUG=1 ./nbd lo -index=0 /tmp/nbd/image.ext4
