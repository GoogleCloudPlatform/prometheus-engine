#!/bin/bash

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
FORK_REPO="${HOME}/Repos/gpe-prometheus"

docker run -it -v "${SCRIPT_DIR}":"/fork" -v "${FORK_REPO}":"/fork_repo" quay.io/bwplotka/copybara:v20250616 \
  migrate /fork/copy.bara.sky -v --init-history

# --init-history
