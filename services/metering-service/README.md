# metering-service

> **Billable-event aggregation.** Consumes `inference.completed.v1`.

`metering-service` is the system of record for what to bill. It
consumes `inference.completed.v1` from the bus, aggregates per
(tenant, model, hour), and exposes:

- `GET /v1/metering/usage?tenant_id=&from=&to=` — usage rollup.
- `GET /v1/metering/invoices` — period invoices.

It also publishes `metering.invoiced.v1` on invoice creation; integrations
(Stripe webhook handler, etc.) consume this.

---

## Why a separate metering tier

Billing is a system-of-record concern. Decoupling metering from the
inference path lets us:

- Replay (Kafka durable subject).
- Re-aggregate (we changed the rounding rule, redo last month).
- Audit (every billable inference is traceable to a frame URN).

---

## See also

- [`../inference-router/README.md`](../inference-router/README.md) — producer.
- [`../cost-accounting/README.md`](../cost-accounting/README.md) — separate, internal-cost view.
