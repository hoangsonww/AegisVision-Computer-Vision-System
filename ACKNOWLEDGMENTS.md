# Acknowledgments

> **Standing on the shoulders of giants.**

AegisVision is an open template assembled from an enormous ecosystem
of open-source work. The lead maintainer is **Son Nguyen**
([@hoangsonww](https://github.com/hoangsonww)); the template itself
stands entirely on the shoulders of the projects below. This file is
a thank-you to the projects, papers, and people whose work made
AegisVision possible.

---

## Open-source projects

### Languages, build, codegen

- **Go** — the Go team.
- **gRPC + Protobuf** — Google + the gRPC contributors.
- **Buf** — Buf Technologies.
- **Task** — Andrey Nering and contributors.

### Runtime + bus

- **NATS / JetStream** — Synadia + the NATS community.
- **Apache Kafka** — the Apache Software Foundation.
- **PostgreSQL** — the PGDG.
- **ClickHouse** — ClickHouse, Inc. + the community.
- **Redis** — Redis Ltd. + the community.

### Cloud-native platform

- **Kubernetes** — the CNCF.
- **Istio** — the Istio community + Google.
- **SPIRE / SPIFFE** — the SPIFFE community.
- **External Secrets Operator** — ESO maintainers.
- **Kyverno** — Nirmata + the community.
- **HashiCorp Vault** — HashiCorp.
- **ArgoCD** — Intuit + the CNCF.

### Supply chain + security

- **sigstore / Cosign / Rekor** — the sigstore community.
- **SLSA** — the OpenSSF SLSA working group.
- **Syft** — Anchore + the community.
- **chaos-mesh** — the chaos-mesh community.

### Observability

- **OpenTelemetry** — the CNCF + the community.
- **Prometheus** — the CNCF + SoundCloud.
- **Grafana / Loki / Tempo** — Grafana Labs + the community.

### GPU + inference

- **NVIDIA Triton Inference Server** — NVIDIA.
- **NVIDIA DeepStream SDK** — NVIDIA.
- **NVIDIA MIG** — NVIDIA.

### Testing + load

- **k6** — Grafana Labs.
- **testify** — the testify contributors.

### Documentation tooling

- **Mermaid** — Knut Sveidqvist + contributors.
- **CommonMark** — the CommonMark community.

---

## Concepts borrowed (and the papers behind them)

- **Wilson's lower-bound proportion test** (canary). Wilson, 1927.
- **Multi-window burn-rate SLO alerting** — Google SRE Workbook,
  Chapter 5 ("Alerting on SLOs").
- **Jensen-Shannon divergence** for distribution drift. Lin, 1991.
- **Bytetrack** for object tracking. Zhang et al., 2022.
- **DeepSORT** — Wojke, Bewley, Paulus, 2017.
- **SLSA framework** for supply-chain provenance. The OpenSSF SLSA
  working group.
- **Bounded autonomy** as an agent-design principle — direct
  scar-tissue from prior CV-platform operational incidents.
- **Claim-check pattern** — Enterprise Integration Patterns (Hohpe +
  Woolf, 2003).
- **RFC 9457 problem+json** — IETF.

---

## Prior art that shaped AegisVision

Direct scar tissue from operating (and watching) other CV platforms
shaped many of the load-bearing decisions:

- The realization that **frames on a queue is always a bug** — every
  CV platform that put frames on Kafka or NATS eventually ripped that
  out. ADR-0008 codifies it.
- The realization that **agents auto-promoting models always leads to
  a regret** — every CV platform that gave agents that power had to
  walk back from at least one incident. ADR-0014 / 0017 codify it.
- The realization that **shared GPUs always melt at the worst
  possible moment** — MIG-default (ADR-0003) is the answer.
- The realization that **air-gapped support has to be day-one** —
  retrofitting it is the slowest path possible. ADR-0027.

These lessons were not learned from this project. The point of
AegisVision is to bake them in from the start.

---

## Thanks to readers + reporters

If you've filed a useful issue, a security report, or a thoughtful
discussion comment — thank you. With your permission, your name will
be added here.

---

## Author

**Son Nguyen** — <hoangson091104@gmail.com>
- GitHub: <https://github.com/hoangsonww>
- LinkedIn: <https://linkedin.com/in/hoangsonw>
- Website: <https://sonnguyenhoang.com>
