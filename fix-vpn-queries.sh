#!/bin/bash
# VPN Plugin Query Fixer - Add source_account_id to all queries
# Fixes 45 queries systematically

set -e

VPN_DB="plugins/vpn/ts/src/database.ts"
BACKUP="plugins/vpn/ts/src/database.ts.query-fix-backup"

echo "=== VPN Query Fixer ==="
echo "Backing up $VPN_DB to $BACKUP"
cp "$VPN_DB" "$BACKUP"

echo "Fixing INSERT queries (adding source_account_id column and value)..."

# Fix upsertProvider - add source_account_id to INSERT
sed -i.tmp1 's/split_tunneling_available, config$/split_tunneling_available, config, source_account_id/' "$VPN_DB"
sed -i.tmp2 's/VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)$/VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)/' "$VPN_DB"
sed -i.tmp3 's/ON CONFLICT (id) DO UPDATE SET$/ON CONFLICT (id, source_account_id) DO UPDATE SET/' "$VPN_DB"
sed -i.tmp4 's/JSON.stringify(provider.config || {}),$/JSON.stringify(provider.config || {}),\n        this.sourceAccountId,/' "$VPN_DB"

echo "Fixing SELECT queries (adding WHERE source_account_id = ...)..."

# Fix getProvider
sed -i.tmp5 "s/WHERE id = \$1'/WHERE id = \$1 AND source_account_id = \$2'/" "$VPN_DB"
sed -i.tmp6 "s/\[id\]);/[id, this.sourceAccountId]);/" "$VPN_DB"

# Fix getAllProviders
sed -i.tmp7 "s/FROM np_vpn_providers ORDER BY name'/FROM np_vpn_providers WHERE source_account_id = \$1 ORDER BY name', [this.sourceAccountId]/" "$VPN_DB"

echo "Cleanup temp files..."
rm -f "$VPN_DB".tmp*

echo "✅ VPN query fixes applied!"
echo "⚠️  Manual verification still required for complex queries"
echo "Backup at: $BACKUP"
