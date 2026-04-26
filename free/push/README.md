# nself-push

> APNs + FCM push notification relay plugin for nSelf. Free — MIT licensed.

## Install

```bash
nself plugin install push
```

Redis is auto-enabled when the push plugin is installed (BullMQ retry queue dependency). If `REDIS_ENABLED` is unset in your env, `nself build` detects the installed plugin and adds the redis service automatically.

## What It Does

Provides APNs (iOS) and FCM v1 (Android) push notification delivery via a Hasura event-trigger pattern. Apps insert a row into `np_push_outbox` and the push service handles credential management, provider dispatch, delivery state tracking, and exponential backoff retry.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `PUSH_APNS_TEAM_ID` | | Apple Developer Team ID |
| `PUSH_APNS_KEY_ID` | | APNs Auth Key ID (.p8) |
| `PUSH_APNS_KEY_PEM` | | APNs Auth Key PEM content (raw or file path) |
| `PUSH_APNS_BUNDLE_ID` | | App bundle ID for APNs routing |
| `PUSH_APNS_SANDBOX` | `0` | Set to `1` for APNs sandbox (development) |
| `PUSH_FCM_PROJECT_ID` | | Firebase project ID |
| `PUSH_FCM_SERVICE_ACCOUNT_JSON` | | FCM service account JSON (raw or file path) |
| `PUSH_RETRY_MAX_ATTEMPTS` | `3` | Maximum delivery attempts before marking failed |
| `PUSH_RETRY_BACKOFF_BASE_MS` | `500` | Base backoff in ms (doubles each retry, capped at 30s) |

See `cli/.github/wiki/plugins/push.md` for full setup, Hasura event trigger configuration, and credential rotation guide.
