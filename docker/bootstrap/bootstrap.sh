#!/bin/sh

test -z "${OAUTH_PW}" && exit 1
test -z "${ADMIN_PW}" && exit 1
test -z "${HOSTNAME}" && exit 1

echo "# bootstrapped application $(date -u -Iseconds)" >> .env
# create storage
ctl bootstrap

# add oauth2 client for littr.me
echo OAUTH_KEY=$(setupkeys.sh "${OAUTH_PW}" "${HOSTNAME}" "${ADMIN_PW}" | grep Client | tail -1 | awk '{print $3}') >> .env
echo OAUTH_PW="${OAUTH_PW}" >> .env
