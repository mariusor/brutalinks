#!/usr/bin/env bash
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
acbuild --debug set-name littr.me/app

# Based on alpine
acbuild --debug dep add quay.io/coreos/alpine-sh

acbuild --debug label add arch amd64
acbuild --debug label add os linux
acbuild --debug label add version "${__version}"

IFS=$'\r\n'
GLOBIGNORE='*'
__env=($(<.env))
for i in ${__env[@]}; do
    name=${i%=*}
    quot=${i#*=}
    value=${quot//\"}
    acbuild environment add "${name}" "${value}"
done

acbuild --debug copy littr /bin/app
acbuild --debug copy-to-dir ./assets /assets
acbuild --debug copy-to-dir ./templates /templates
acbuild --debug set-exec /bin/app

# Add a port for http traffic over port 3000
acbuild --debug port add www tcp 3000

acbuild --debug set-working-directory /

acbuild --debug annotation add authors "Marius Orcsik <marius@littr.me>"

# Save the ACI
acbuild --debug write --overwrite "littr-me-app-${__version}-linux-amd64.aci"
