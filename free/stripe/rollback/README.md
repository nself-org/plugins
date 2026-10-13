# rollback

`001_rename_stripe_to_np_stripe_down.sql` reverts the `np_stripe_*` rename. It is a dev and staging rollback
script, not a forward migration: it sits outside `migrations/` so the boot runner (sdk/go/migrate) never applies it.
Run it by hand only, with a fresh dump taken first.
