#!/bin/sh
set -e

# Ensure machine-id exists (required by D-Bus)
if [ ! -f /etc/machine-id ]; then
  dbus-uuidgen > /etc/machine-id
fi

# Start D-Bus system bus (required by avahi-daemon)
mkdir -p /run/dbus
dbus-daemon --system --fork

# Brief pause to let dbus initialize
sleep 0.3

# Start avahi-daemon (provides mDNS for shairport-sync AirPlay advertisement).
# --no-chroot is required inside Docker containers.
mkdir -p /run/avahi-daemon
avahi-daemon --daemonize --no-chroot

# Brief pause to let avahi initialize
sleep 0.3

exec "$@"
