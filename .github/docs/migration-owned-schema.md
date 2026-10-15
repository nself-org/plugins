# Migration-owned schema

Torrent-manager, VPN and web3 apply their embedded SQL migrations before serving
requests. Their runtime database wrappers do not create tables, indexes or views.
Schema changes belong in a new numbered migration; existing published migrations
must retain their checksums.

VPN migration `002_views.sql` adds the four reporting views in `public`, matching
their original column order, account-scoped joins and calculations. `CREATE OR
REPLACE VIEW` also supports databases where the previous runtime initializer
already created these views. Both destinations and tables are qualified because
the migration runner sets a plugin-specific search path.

Run the regression guards with `python3 tests/boot/test_migration_schema.py` and
the fresh database checks with
`bash tests/boot/run.sh --plugins torrent-manager,vpn,web3`.

The required lint checks also remove unused torrent RPC metadata and make ignored
response cleanup errors explicit. Web3 handlers are split into files below the
repository size limit. Web3 request logs now use the socket peer address, after
removing deprecated middleware that trusted unverified forwarded IP headers.
