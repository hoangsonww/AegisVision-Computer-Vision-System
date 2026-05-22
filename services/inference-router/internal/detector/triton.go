// Package detector wraps Triton Inference Server behind the dataplane
// Detector interface.
//
// Triton's HTTP/REST protocol (KServe v2 inference protocol) is the wire
// contract; this client speaks it directly so we don't carry the Triton
// gRPC SDK as a build-time dep across every operator pod. The shape is:
//
//	POST {URL}/v2/models/{model_name}/versions/{ver}/infer
//	{"inputs":[{"name":"images","shape":[1,3,640,640],"datatype":"FP32",...}],
//	 "outputs":[{"name":"detections"}]}
//
// The Phase 2 implementation will switch to gRPC streaming + shared memory
// extensions. The interface (dataplane/operators.Detector) does not change.
package detector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

// TritonClient implements dataplane operators.Detector against Triton's
// HTTP API. The payload semantics (what bytes mean) are agreed with the
// upstream operator that wrote them into the claim-check ring — for the
// production path that's pre-processed CHW float32 image data.
type TritonClient struct {
	BaseURL      string
	ModelName    string
	ModelVersion string
	HTTPClient   *http.Client
}

func NewTriton(baseURL, modelName, modelVersion string) *TritonClient {
	return &TritonClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		ModelName: modelName,
		ModelVersion: modelVersion,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *TritonClient) ModelID() string        { return c.ModelName }
func (c *TritonClient) ModelVersionID() string { return c.ModelVersion }

// Detect calls Triton with the raw payload bytes. The on-wire request is
// the KServe v2 inference protocol; the response shape depends on the
// model. We try a small set of common output formats and fall through to
// raising an error on unknown shapes so misconfiguration is loud.
func (c *TritonClient) Detect(ctx context.Context, frame *dataplanev1.FrameDescriptor, payload []byte) ([]*dataplanev1.Detection, error) {
	url := fmt.Sprintf("%s/v2/models/%s/versions/%s/infer",
		c.BaseURL, c.ModelName, c.ModelVersion)
	body := tritonRequest{
		Inputs: []tritonTensor{{
			Name:     "input",
			Datatype: "BYTES",
			Shape:    []int{1},
			Data:     []any{string(payload)},
		}},
		Outputs: []tritonOutput{{Name: "output"}},
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("triton: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("triton: status %d: %s", resp.StatusCode, string(b))
	}
	var out tritonResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("triton: decode: %w", err)
	}
	return out.toDetections(frame, c.ModelName, c.ModelVersion)
}

// --- KServe v2 wire types ---

type tritonTensor struct {
	Name     string `json:"name"`
	Datatype string `json:"datatype"`
	Shape    []int  `json:"shape"`
	Data     []any  `json:"data"`
}

type tritonOutput struct {
	Name string `json:"name"`
}

type tritonRequest struct {
	Inputs  []tritonTensor `json:"inputs"`
	Outputs []tritonOutput `json:"outputs"`
}

type tritonResponse struct {
	ModelName    string         `json:"model_name"`
	ModelVersion string         `json:"model_version"`
	Outputs      []tritonTensor `json:"outputs"`
}

// toDetections projects a Triton response onto the platform's Detection
// schema. We support two output shapes:
//
//	1. JSON-encoded list of {class, score, bbox{x,y,w,h}} entries —
//	   common for custom Triton Python backends.
//	2. A flat FP32 tensor of [N x 6] = [x, y, w, h, score, class_id] —
//	   what Triton TensorRT outputs for detection models.
func (r *tritonResponse) toDetections(frame *dataplanev1.FrameDescriptor, modelID, modelVer string) ([]*dataplanev1.Detection, error) {
	if len(r.Outputs) == 0 {
		return nil, fmt.Errorf("triton: empty outputs")
	}
	out := r.Outputs[0]

	// Shape 1: JSON-encoded list, surfaced as a single BYTES element.
	if out.Datatype == "BYTES" && len(out.Data) == 1 {
		if s, ok := out.Data[0].(string); ok {
			var items []struct {
				Class string  `json:"class"`
				Score float32 `json:"score"`
				BBox  struct {
					X, Y, W, H float32
				} `json:"bbox"`
			}
			if err := json.Unmarshal([]byte(s), &items); err == nil {
				dets := make([]*dataplanev1.Detection, 0, len(items))
				for _, it := range items {
					dets = append(dets, &dataplanev1.Detection{
						StreamId: frame.GetStreamId(), PipelineId: frame.GetPipelineId(),
						FrameSeq: frame.GetFrameSeq(), CaptureTime: frame.GetCaptureTime(),
						ModelId: modelID, ModelVersionId: modelVer,
						ClassLabel: it.Class, Score: it.Score,
						Bbox: &dataplanev1.BoundingBox{X: it.BBox.X, Y: it.BBox.Y, W: it.BBox.W, H: it.BBox.H},
					})
				}
				return dets, nil
			}
		}
	}

	// Shape 2: flat FP32 [N x 6].
	if out.Datatype == "FP32" && len(out.Shape) == 2 && out.Shape[1] == 6 {
		n := out.Shape[0]
		if len(out.Data) != n*6 {
			return nil, fmt.Errorf("triton: FP32 shape mismatch")
		}
		dets := make([]*dataplanev1.Detection, 0, n)
		for i := 0; i < n; i++ {
			f := func(idx int) float32 {
				v, _ := out.Data[i*6+idx].(float64)
				return float32(v)
			}
			dets = append(dets, &dataplanev1.Detection{
				StreamId: frame.GetStreamId(), PipelineId: frame.GetPipelineId(),
				FrameSeq: frame.GetFrameSeq(), CaptureTime: frame.GetCaptureTime(),
				ModelId: modelID, ModelVersionId: modelVer,
				ClassLabel: fmt.Sprintf("class-%d", int(f(5))),
				Score:      f(4),
				Bbox:       &dataplanev1.BoundingBox{X: f(0), Y: f(1), W: f(2), H: f(3)},
			})
		}
		return dets, nil
	}

	return nil, fmt.Errorf("triton: unrecognized output shape %s/%v", out.Datatype, out.Shape)
}
