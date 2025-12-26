#!/usr/bin/env sh
TARGET="${1}"
APP_HOSTNAME=$(basename "${TARGET}")

openssl req \
  -subj "/C=AQ/ST=Omond/L=Omond/O=${APP_HOSTNAME}/OU=none/CN=${APP_HOSTNAME}" \
  -newkey rsa:2048 -sha256 \
  -keyout "${TARGET}.key" \
  -nodes -x509 -days 365 \
  -out "${TARGET}.crt" && \
cat "${TARGET}.key" "${TARGET}.crt" > "${TARGET}.pem"
