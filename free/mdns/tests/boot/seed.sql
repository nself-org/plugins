-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Test data only.
INSERT INTO np_mdns_services (id, service_name, port) VALUES ('00000000-0000-4000-8000-0000000000d1', 'seed-service', 8080);
INSERT INTO np_mdns_discovery_log (id, service_type, service_name, host, port) VALUES ('00000000-0000-4000-8000-0000000000d2', '_ntv._tcp', 'seed-service', 'seed.local', 8080);
