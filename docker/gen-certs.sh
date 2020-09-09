#!/usr/bin/env sh
APP_HOSTNAME="${1}"

openssl req \
  -subj "/C=AQ/ST=Omond/L=Omond/O=${APP_HOSTNAME}/OU=none/CN=${APP_HOSTNAME}" \
  -newkey rsa:2048 -sha256 \
  -keyout "${APP_HOSTNAME}.key" \
  -nodes -x509 -days 365 \
  -out "${APP_HOSTNAME}.crt" && \
cat "${APP_HOSTNAME}.key" "${APP_HOSTNAME}.crt" > "${APP_HOSTNAME}.pem"
