// Temporal-backed Deployer. We don't pull in the Temporal SDK directly
// — that's a large dependency, configured via Helm when used. Instead
// this file gives us:
//
//   - the TemporalDeployer struct that satisfies the Deployer interface,
//   - a concrete HTTP-call to Temporal's frontend gRPC gateway, OR the
//     fallback NoopDeployer when no host is configured.
//
// The wire shape (StartWorkflowExecution) is the standard Temporal Frontend
// gRPC contract; we use the JSON gateway here to keep this file self-
// contained. Production charts can swap in the official SDK by replacing
// the New() constructor — the Deployer interface is unchanged.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TemporalDeployer dispatches "DeployPipeline" workflows to a Temporal
// cluster via its HTTP gateway.
type TemporalDeployer struct {
	Frontend  string // http://temporal.aegis-system.svc:7243
	Namespace string
	TaskQueue string
	WorkflowType string // canonical name registered in workers
	HTTPClient   *http.Client
}

// NewTemporalDeployer constructs a Deployer that talks to a Temporal
// frontend at the given URL. Returns nil + nil when frontend == "" so the
// caller can fall back to NoopDeployer.
func NewTemporalDeployer(frontend, ns, taskQueue, workflowType string) *TemporalDeployer {
	if frontend == "" {
		return nil
	}
	if ns == "" {
		ns = "aegisvision"
	}
	if taskQueue == "" {
		taskQueue = "pipeline-deploy"
	}
	if workflowType == "" {
		workflowType = "DeployPipelineWorkflow"
	}
	return &TemporalDeployer{
		Frontend: frontend, Namespace: ns, TaskQueue: taskQueue, WorkflowType: workflowType,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// StartDeploy POSTs a StartWorkflowExecution request to Temporal.
func (t *TemporalDeployer) StartDeploy(ctx context.Context, pipelineID string) (string, error) {
	wfID := "deploy-" + pipelineID + "-" + newID()
	body := map[string]any{
		"workflow_id":     wfID,
		"workflow_type":   map[string]string{"name": t.WorkflowType},
		"task_queue":      map[string]string{"name": t.TaskQueue},
		"input":           map[string]any{"pipeline_id": pipelineID},
		"workflow_id_reuse_policy": "WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE",
	}
	buf, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/api/v1/namespaces/%s/workflows", t.Frontend, t.Namespace)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("temporal: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("temporal: status %d: %s", resp.StatusCode, string(b))
	}
	return wfID, nil
}

// SelectDeployer returns a TemporalDeployer when frontend is configured,
// otherwise NoopDeployer. This is the canonical constructor pipeline-service
// uses at startup.
func SelectDeployer(frontend, namespace, taskQueue, workflowType string) Deployer {
	if d := NewTemporalDeployer(frontend, namespace, taskQueue, workflowType); d != nil {
		return d
	}
	return NoopDeployer{}
}

var _ = errors.New // keep errors import even after future edits
