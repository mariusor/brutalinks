#!/bin/sh

if [[ -z "${FEDBOX_HOSTNAME}" ]]; then
    echo "Missing fedbox hostname in environment";
    exit 1
fi
if [[ -z "${OAUTH2_SECRET}" ]]; then
    echo "Missing OAuth2 password in environment";
    exit 1
fi
if [[ -z "${OAUTH2_CALLBACK_URL}" ]]; then
    echo "Missing OAuth2 callback url in environment";
    exit 1
fi

_ENV_FILE=/.env

if [ ! -f "${_ENV_FILE}" ]; then
    echo "Invalid .env file ${_ENV_FILE}"
    exit 1
fi

_FULL_PATH="${STORAGE_PATH}/${ENV}/${FEDBOX_HOSTNAME}"
if [[ -d "${_FULL_PATH}" ]]; then
    echo "skipping bootstrapping ${_FULL_PATH}"
else
    echo "# bootstrapped application $(date -u -Iseconds)" >> ${_ENV_FILE}
    # create storage
    ctl bootstrap
fi

_HAVE_OAUTH2_SECRET=$(grep OAUTH2_SECRET "${_ENV_FILE}" | cut -d'=' -f2 | tail -n1)
_HAVE_OAUTH2_CLIENT=$(ctl oauth client ls | grep -c "${OAUTH2_KEY}")

if [[ ${_HAVE_OAUTH2_CLIENT} -ge 1 && "z${_HAVE_OAUTH2_SECRET}" == "z${OAUTH2_SECRET}" ]]; then
    echo "skipping adding OAuth2 client"
else
    # add oauth2 client for littr.me
    echo OAUTH2_KEY=$(clientadd.sh "${OAUTH2_SECRET}" "${OAUTH2_CALLBACK_URL}" | grep Client | tail -1 | awk '{print $3}') >> .env
    echo OAUTH2_SECRET="${OAUTH2_SECRET}" >> .env
fi

_ADMIN_NAME=admin
_HAVE_ADMIN=$(ctl ap ls --type Person | jq -r .[].preferredUsername | grep -c "${_ADMIN_NAME}")
if [[ ${_HAVE_ADMIN} -ge 1  ]]; then
    echo "skipping adding user ${_ADMIN_NAME}"
else
    if [[ -n "${ADMIN_PW}" ]]; then
        # add admin user for littr.me
        useradd.sh "${_ADMIN_NAME}" "${ADMIN_PW}"
    fi
fi
