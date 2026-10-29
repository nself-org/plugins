#!/usr/bin/env bash
set -eu
. "$(dirname "$0")/redis.sh"
list=feature-flags; redis=test-redis; net=test-network
calls=()
docker() { calls+=("$*"); }
boot_redis_start
[ ${#calls[@]} -eq 0 ]
list=event-bus,job-queue,functions-v8
boot_redis_start
[ "${calls[0]}" = 'run -d --name test-redis --network test-network --network-alias redis redis:7-alpine' ]
[ "${calls[1]}" = 'exec test-redis redis-cli ping' ]
docker() { return 1; }
if boot_redis_start; then echo 'FAIL: Redis startup failure was ignored' >&2; exit 1; fi
printf 'boot Redis sidecar: ok\n'
