#!/bin/bash

podman ps -q --filter name=tests_ | xargs podman kill
