#!/bin/bash
set -ax

. .env.Docker || exit 1
set +ax

make -C docker/ images
