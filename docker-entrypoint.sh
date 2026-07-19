#!/bin/sh
set -e

# Start D-Bus (required by libavahi-client for mDNS)
mkdir -p /run/dbus
dbus-daemon --system --nofork &
for i in $(seq 30); do
    [ -S /run/dbus/system_bus_socket ] && break
    sleep 0.1
done

# Start avahi-daemon for mDNS (shairport-sync AirPlay discovery)
mkdir -p /run/avahi-daemon
avahi-daemon --no-chroot -D
for i in $(seq 30); do
    [ -S /run/avahi-daemon/socket ] && break
    sleep 0.1
done

exec "$@"
