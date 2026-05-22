# ADR-0002: ClickHouse for the detection firehose, PostgreSQL for metadata

- Status: Accepted
- Date: 2026-05-17

## Context

A single 10,000-stream deployment at 30 fps is 300,000 detection events per
second at steady state, and bursts during incidents can multiply that by an
order of magnitude. PostgreSQL alone cannot ingest, store, or query that
volume at a price point that survives a tenancy budget review.

## Decision

Two stores, each used for what it is good at:

- **PostgreSQL 16** holds the *metadata* — tenants, projects, pipelines,
  models, cameras, users, policies, audit summaries. Transactional, RI,
  reasonable row counts.
- **ClickHouse** holds the *firehose* — detections.v1, tracks.v1, events.v1,
  ocr.results.v1, gpu.telemetry.v1, audit.v1. Append-only, partitioned by
  (tenant, day), TTL'd by retention.

Joins across the two are forbidden in the hot path. ClickHouse rows carry
the tenant/project/pipeline/model IDs verbatim from Postgres; the console
denormalizes for display.

## Consequences

- A "give me the last 1,000 detections for stream X" query is fast and
  bounded; an "ALTER TABLE detections" is impossible at scale and is
  therefore designed-against.
- Retention is enforced by ClickHouse partition drops. Per-tenant retention
  windows fit naturally; long-tail per-stream overrides are reference-
  counted and cascade across stores.
- The cost of a new analytic is the cost of an OLAP query, not the cost of
  another B-tree index on the OLTP cluster.
