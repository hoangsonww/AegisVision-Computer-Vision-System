# annotation-service

> **Labels + label-policy revisions.**

Annotations are the supervised signal that trains models. `annotation-service`
owns the label-policy (the schema of what classes/attributes exist),
its revisions, and the individual labels attached to samples in
`dataset-service`.

---

## API

```
POST /v1/label-policies               Create.
GET  /v1/label-policies/{id}          Read.
POST /v1/label-policies/{id}/revisions  Cut a new revision (immutable).
POST /v1/annotations                  Create label(s).
GET  /v1/annotations                  Cursor-paginated.
PATCH /v1/annotations/{id}            Update (creates a new version).
DELETE /v1/annotations/{id}           Soft delete.
```

Label policies are **immutable per revision**. Changing a class label
("car" → "vehicle") creates a new revision; models trained on the old
revision continue to refer to the old labels.

---

## Why immutable revisions

Because a model trained against "car" is meaningless if you later
rename the class. Lineage requires that the label space at training
time is recoverable.

---

## See also

- [`../dataset-service/README.md`](../dataset-service/README.md).
- [`../training-orchestrator/README.md`](../training-orchestrator/README.md).
