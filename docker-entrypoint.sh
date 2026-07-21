#!/bin/sh
set -e

# Wait for a non-loopback network interface to be ready.
# tinysvcmdns needs to join the multicast group (224.0.0.251) at startup;
# if the host network isn't up yet, IP_ADD_MEMBERSHIP fails with ENODEV.
for i in $(seq 30); do
    # Check for any non-loopback interface with an assigned IPv4 address
    ip addr show | grep -v '127\.' | grep -q 'inet ' && break
    sleep 1
done

exec "$@"
