#!/bin/sh
set -e

# Start avahi-daemon for mDNS advertisement (shairport-sync AirPlay discovery).
# --no-dbus: avahi runs without D-Bus (avoids dbus socket issues in Docker).
# --no-chroot: required inside Docker containers.
# -D: daemonize.
mkdir -p /run/avahi-daemon
avahi-daemon --no-dbus --no-chroot -D

exec "$@"
