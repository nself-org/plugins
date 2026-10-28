#!/usr/bin/env bash
# Shared by both boot kits. Only job-queue needs Redis; no host ports are exposed.
boot_redis_start() {
  case ",$list," in *,job-queue,*) ;; *) return 0;; esac
  docker run -d --name "$redis" --network "$net" --network-alias redis redis:7-alpine >/dev/null || return 1
  local attempt
  for attempt in $(seq 1 30); do
    if docker exec "$redis" redis-cli ping >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  echo 'FAIL boot: Redis sidecar did not become ready' >&2
  return 1
}
