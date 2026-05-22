# Disaster recovery runbook

Per architecture doc §33, our DR targets are:

| Tier | RPO | RTO |
| --- | --- | --- |
| Postgres metadata | 5 min | 30 min |
| ClickHouse firehose | 1 min | 1 hour |
| Object storage | ~0 | 15 min |
| Kafka | 1 min | 30 min |

## Postgres (control plane) restore

1. Confirm WAL archives are intact in `s3://aegis-postgres-wal/`.
2. Provision a fresh Postgres cluster: `terraform apply -target module.postgres`.
3. Restore base backup + replay WAL: `pg_restore` + `pg_wal_replay_resume`.
4. Re-point services: update the `model-registry-db` / `audit-db` /
   `pipeline-db` ExternalSecrets in Vault, force a refresh.
5. Verify migrations are at HEAD: `select max(version) from schema_migrations`.

## ClickHouse rebuild from Kafka

1. ClickHouse is rebuildable from the Kafka `detections.v1` + `events.v1`
   topics. Spin up a fresh CH cluster.
2. Run the schema migrations (`MigratePersistent` with `isClickHouse=true`).
3. Start the consumer with `--from-beginning` on the highest-priority topic
   (events.v1 first; backfill detections.v1 in parallel).
4. Catchup completion is observable via the consumer lag dashboard.

## Region failure

1. The platform is active-active across regions. Promote the secondary's
   `Stream` records to primary via `kubectl annotate stream <id>
   aegisvision.io/promoted=true`. The dataplane-runner watches the
   annotation.
2. DNS failover: switch the `api.aegisvision.example` Route53 record from
   the primary's NLB to the secondary's.
3. Verify glass-to-event SLO in the new primary before declaring incident
   resolved.

## Tenant data deletion (GDPR / right-to-erasure)

1. Trigger the deletion workflow: `aegis-cli tenant delete --id <tenant>`.
2. The workflow:
   a) Stops all pipelines for the tenant,
   b) Crypto-shreds the per-tenant key in Vault (cascading deletes follow
      automatically: bytes encrypted with that key become unreadable),
   c) Drops ClickHouse partitions for `tenant_id = <id>`,
   d) Issues object-storage delete-marker for `s3://aegis-media/<tenant>/*`,
   e) Writes an `audit.v1` `TenantDeleted` record (kept for compliance).
3. Cross-region replicas pick up the deletion within 15 minutes via the
   audit-service consumer.
