#!/bin/sh
set -e

# Start avahi-daemon for mDNS advertisement (shairport-sync AirPlay discovery).
# enable-dbus=no is set in /etc/avahi/avahi-daemon.conf (no dbus in this container).
# --no-chroot: required inside Docker containers.
# -D: daemonize.
mkdir -p /run/avahi-daemon
avahi-daemon --no-chroot -D

exec "$@"
