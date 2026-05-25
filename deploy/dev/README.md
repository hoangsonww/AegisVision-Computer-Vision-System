# Local development backing services

This directory holds the `docker-compose.yml` that brings up the full
local dev stack: Postgres, Redis, NATS JetStream, Kafka (KRaft),
ClickHouse, MinIO, and an OpenTelemetry collector. Everything the
production-shape AegisVision services need in one command.

## Bring it up

From the repo root:

```bash
cd deploy/dev
docker compose up -d
docker compose ps                  # wait for "healthy" on every container
```

The bootstrap container creates four buckets in MinIO
(`aegis-claimcheck`, `aegis-models`, `aegis-recordings`, `aegis-evidence`)
and exits cleanly — that's expected.

## Connection URLs

Export these in any shell that runs `task run:<service>`:

```bash
export AEGIS_NATS_URL=nats://127.0.0.1:4222
export AEGIS_KAFKA_BROKERS=127.0.0.1:9092
export AEGIS_POSTGRES_DSN='postgres://aegis:aegis@127.0.0.1:5432/aegis?sslmode=disable'
export AEGIS_REDIS_ADDR=127.0.0.1:6379
export AEGIS_CLICKHOUSE_URL=tcp://default:@127.0.0.1:9000/aegis
export AEGIS_S3_ENDPOINT=http://127.0.0.1:9100
export AEGIS_S3_ACCESS_KEY=aegisadmin
export AEGIS_S3_SECRET_KEY=aegisadmin123
export AEGIS_OTLP_ENDPOINT=127.0.0.1:4317
```

## Web consoles

| Service | URL | Credentials |
| --- | --- | --- |
| MinIO console | http://127.0.0.1:9101 | `aegisadmin` / `aegisadmin123` |
| NATS monitoring | http://127.0.0.1:8222/ | — |
| ClickHouse HTTP | http://127.0.0.1:8123/play | `default` / empty |
| pgAdmin (optional) | http://127.0.0.1:5050 | `dev@aegisvision.local` / `aegis` |

The pgAdmin container is behind a profile — start it explicitly:

```bash
docker compose --profile tools up -d pgadmin
```

## Tear down

```bash
docker compose down -v       # also wipes volumes
```

Drop `-v` to keep persistent volumes between runs.

## Notes

- This stack has **no auth between services**, **no TLS**, and **no
  multi-replica HA**. It is for development only — never deploy this
  configuration anywhere reachable from the internet.
- Resource footprint: ~3 GiB RAM with all containers healthy. ClickHouse
  and Kafka are the heaviest.
- The OTel collector writes traces and metrics to its own stdout —
  `docker logs aegis-dev-otel` is the dev "tracing UI".
- Recipes for seeding the platform with demo tenants / pipelines live
  at `tools/scripts/seed-dev.sh` (separate from this directory so it
  can target any environment).

## Related

- [`SETUP_GUIDE.md`](../../SETUP_GUIDE.md) — the canonical setup guide,
  including the production cluster + air-gap paths.
- [`TEMPLATE.md`](../../TEMPLATE.md) — adoption playbook for forking
  the template.
