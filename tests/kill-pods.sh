#!/bin/bash

podman kill $(podman ps -q --filter name=tests_)
