# Example Triton model repository

A minimal Triton model-repository layout that the platform's
`model-registry` can load. This is the "Hello, Triton" for an
AegisVision adopter — replace the synthetic Python backend with your
real TensorRT engine when you're ready.

```
examples/triton-repo/
├── README.md
├── person-detector/             # TensorRT detection model (placeholder)
│   ├── config.pbtxt
│   └── 1/
│       └── README.md            # drop your model.plan here
├── synthetic-detector/          # Python backend — works without GPU
│   ├── config.pbtxt
│   └── 1/
│       └── model.py
└── detect-then-blur/            # Ensemble linking detector + blur op
    └── config.pbtxt
```

## Loading these into Triton

Upload the directory to your bucket:

```bash
aws s3 sync deploy/helm/triton/examples/triton-repo \
  s3://yourorg-models/triton-repo
```

The Triton chart's init container syncs the bucket onto each pod's
`/models` volume on startup. With `--model-control-mode=explicit`
(the production default) no model is loaded until `model-registry`
issues a `POST /v2/repository/models/{name}/load`.

## What's in each model

### `synthetic-detector`
A Python-backend model that returns a deterministic single detection
for any input. Useful for end-to-end testing without GPUs — the
walking-skeleton uses this in dev. It demonstrates: input/output
schemas, dynamic batching, model warmup, response cache disabled
(per-frame detector).

### `person-detector`
A placeholder for a TensorRT person-detection model (YOLOv8 family).
The directory ships the `config.pbtxt` shape; drop your compiled
`model.plan` into `1/` and you have a working production detector.

### `detect-then-blur`
An ensemble that demonstrates linking two models into a single
inference call: `person-detector` finds boxes → a (notional) blur
operator redacts them. This is the pattern for privacy-preserving
detection pipelines.

## Adapting for your fleet

- If your fleet has H100 / B100 GPUs, recompile the TensorRT engine
  for that SKU with `trtexec`.
- For non-TensorRT models, swap the `platform:` line in `config.pbtxt`
  to `onnxruntime_onnx`, `pytorch_libtorch`, or `tensorflow_savedmodel`.
- For per-tenant model isolation, add a per-tenant `instance_group`
  with the tenant ID label.

See [`docs/triton.md`](../../../../docs/triton.md) for the full
operating manual.
