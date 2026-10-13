# rollback

`001_rename_stripe_to_np_stripe_down.sql` reverts the `np_stripe_*` rename. It is a dev and staging rollback
script, not a forward migration: it sits outside `migrations/` so the boot runner (sdk/go/migrate) never applies it.
Run it by hand only, with a fresh dump taken first.

The boot runner records the rename in `np_stripe.schema_migrations`. A rollback that leaves that row behind makes the
next start skip the rename, and `InitSchema` then creates empty `np_stripe_*` tables beside the restored `stripe_*`
ones. So roll back in this order: stop the plugin, run `001_rename_stripe_to_np_stripe_down.sql`, then
`DELETE FROM np_stripe.schema_migrations WHERE filename = '001_rename_stripe_to_np_stripe_up.sql';`, then start the
previous plugin version.
