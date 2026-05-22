# dataset-service

> **Datasets + dataset versions + lineage.**

A dataset is a named collection of samples (frames, clips, or
synthetic). Datasets have **versions**; each version is immutable.
Lineage records *which* dataset version a model was trained on, who
labeled which sample, and where each sample came from (which stream,
which frame URN).

---

## API

```
POST /v1/datasets                Create.
GET  /v1/datasets                List.
GET  /v1/datasets/{id}           Read.
POST /v1/datasets/{id}/versions  Cut a new version (immutable).
GET  /v1/datasets/{id}/versions/{ver}/samples    Cursor-paginated.
POST /v1/datasets/{id}/versions/{ver}/samples    Add samples.
```

---

## Lineage

Every sample carries:

- `source_stream_id` (where it came from).
- `frame_urn` (claim-check reference).
- `created_by` (user or active-learning-service).
- `labels` (via annotation-service).

When a model is trained, `training-orchestrator` records the dataset
version. When the model is deployed, you can answer "what data taught
this model?" by walking the lineage.

---

## See also

- [`../annotation-service/README.md`](../annotation-service/README.md).
- [`../training-orchestrator/README.md`](../training-orchestrator/README.md).
- [`../active-learning-service/README.md`](../active-learning-service/README.md).
