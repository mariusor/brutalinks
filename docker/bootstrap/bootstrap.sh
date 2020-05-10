#!/bin/sh

# create boltdb
/bin/ctl bootstrap

test -z ${OAUTH_PW} && exit 1
test -z ${ADMIN_PW} && exit 1

# add oauth2 client for littr.me
/bin/ctl oauth client add --redirectUri http://${HOSTNAME}/auth/fedbox/callback << ${OAUTH_PW} \
${OAUTH_PW}

# add admin user
/bin/ctl actor add admin << ${ADMIN_PW} \
${ADMIN_PW}


