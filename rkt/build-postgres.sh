#!/usr/bin/env bash
set -e

if [ "$EUID" -ne 0 ]; then
    echo "This script uses functionality which requires root privileges"
    exit 1
fi

# Start the build with an empty ACI
acbuild --debug begin

# In the event of the script exiting, end the build
acbuildEnd() {
    export EXIT=$?
    acbuild --debug end && exit $EXIT
}
trap acbuildEnd EXIT

__version=$(printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)")
# Name the ACI
acbuild --debug set-name littr.me/postgres

# Based on alpine
acbuild --debug dep add quay.io/coreos/alpine-sh

acbuild --debug label add arch amd64
acbuild --debug label add os linux
acbuild --debug label add version "${__version}"

# Install postgres
acbuild --debug run apk update
acbuild --debug run apk add postgresql

acbuild --debug set-user postgres

# Add a mount point for files to serve
acbuild --debug mount add data /data
acbuild --debug mount add sock /var/run/postgresql

# Run postgres in the foreground
acbuild --debug set-exec -- /usr/bin/postgres -D /data

acbuild annotation add authors "Marius Orcsik <marius@littr.me>"

# Save the ACI
acbuild --debug write --overwrite "littr-me-postgres-${__version}-linux-amd64.aci"

