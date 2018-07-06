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
acbuild --debug set-name littr.me/nginx

# Based on alpine
acbuild --debug dep add quay.io/coreos/alpine-sh

acbuild --debug label add arch amd64
acbuild --debug label add os linux
acbuild --debug label add version "${__version}"

# Install nginx
acbuild --debug run apk update
acbuild --debug run apk add nginx

acbuild copy rkt/nginx/nginx.conf /etc/nginx/nginx.conf

# Add a port for http traffic over port 80
acbuild --debug port add http tcp 80

# Add a mount point for files to serve
#acbuild --debug mount add html /usr/share/nginx/html

# Run nginx in the foreground
acbuild --debug set-exec -- /usr/sbin/nginx -g "daemon off;"

acbuild annotation add authors "Marius Orcsik <marius@littr.me>"

# Save the ACI
acbuild --debug write --overwrite "littr-me-nginx-${__version}-linux-amd64.aci"

