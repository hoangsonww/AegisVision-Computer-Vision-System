# Place your model artifact here

This directory should contain `model.plan` — the TensorRT engine
compiled for your target GPU SKU.

Compile from an ONNX export with `trtexec` on the same GPU family
that Triton will serve from:

```bash
trtexec \
  --onnx=person-detector.onnx \
  --saveEngine=model.plan \
  --fp16 \
  --workspace=4096 \
  --minShapes=images:1x3x640x640 \
  --optShapes=images:4x3x640x640 \
  --maxShapes=images:16x3x640x640
```

Re-compile per GPU SKU — a `model.plan` built on an A100 will not
load on an H100, and vice versa.
