#!/usr/bin/env bash
set -Eeuxo pipefail
cd "$(dirname ${BASH_SOURCE[0]})"

export PROGRAM="./shell-target-exp config.json"


echo yo | $PROGRAM ok!
