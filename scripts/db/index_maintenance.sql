-- Index maintenance helper for post-deployment verification.
-- Run this script after applying migrations/0023_performance_indexes.sql
-- to refresh optimizer statistics and validate index usage per tenant shards.

SET SESSION sql_notes = 0;

ANALYZE TABLE address_pools,
             static_bindings,
             leases_v4,
             prefix_leases_v6;

-- Optional: run during low-traffic windows to defragment pages touched by new indexes.
OPTIMIZE TABLE leases_v4,
              prefix_leases_v6;

SET SESSION sql_notes = DEFAULT;
