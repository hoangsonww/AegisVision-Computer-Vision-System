# Synthetic detector — a Python-backend Triton model that returns a
# fixed "person at (0.5, 0.5)" detection for any input. This mirrors
# the in-process SyntheticDetector at
# pkg/dataplane/operators/detector.go so the same end-to-end flow can
# be exercised through a real Triton instance.
#
# Triton loads this file via its Python backend (--backend-config=python,...).
# The class name `TritonPythonModel` and the three lifecycle methods
# (`initialize`, `execute`, `finalize`) are the Triton Python API
# contract. See https://github.com/triton-inference-server/python_backend.

import json

import numpy as np
import triton_python_backend_utils as pb_utils


class TritonPythonModel:
    """Synthetic detector — deterministic 'person' detection for any input."""

    def initialize(self, args):
        """Run once per Triton pod when the model is loaded."""
        self.model_config = json.loads(args["model_config"])
        # output is a BYTES tensor with a JSON-encoded detection list.
        self.output_dtype = np.bytes_

    def execute(self, requests):
        """Run once per inference request. `requests` is a batch."""
        responses = []
        for _ in requests:
            payload = json.dumps([
                {
                    "class": "person",
                    "score": 0.95,
                    "bbox": {"X": 0.5, "Y": 0.5, "W": 0.1, "H": 0.1},
                },
            ]).encode("utf-8")
            output = pb_utils.Tensor(
                "output",
                np.array([payload], dtype=self.output_dtype),
            )
            responses.append(pb_utils.InferenceResponse(output_tensors=[output]))
        return responses

    def finalize(self):
        """Run once when the model is unloaded."""
        pass
