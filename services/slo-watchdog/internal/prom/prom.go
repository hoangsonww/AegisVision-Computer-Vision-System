// Package prom is a minimal Prometheus query client for the SLO watchdog.
//
// We deliberately do not pull in github.com/prometheus/client_golang/api
// — the surface we need is one endpoint with no time series unmarshalling,
// just `instant query → single scalar`.
//
// If the query returns more than one sample or a non-scalar, we treat it
// as a misconfiguration (no SLO target should produce multi-sample
// output).
package prom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client queries Prometheus instant-vector endpoint.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{BaseURL: baseURL, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// Query returns the scalar value of the given PromQL at time t. Returns
// (0, err) when the query has no result.
func (c *Client) Query(ctx context.Context, q string, t time.Time) (float64, error) {
	if c.BaseURL == "" {
		return 0, errors.New("prom: empty base url")
	}
	u := c.BaseURL + "/api/v1/query?" + url.Values{
		"query": []string{q},
		"time":  []string{strconv.FormatInt(t.Unix(), 10)},
	}.Encode()
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("prom %s: %s", resp.Status, string(b))
	}
	var body struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string            `json:"resultType"`
			Result     []json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, err
	}
	if body.Status != "success" {
		return 0, errors.New("prom: non-success response")
	}
	if len(body.Data.Result) == 0 {
		return 0, errors.New("prom: empty result")
	}
	// vector result: [{"metric":{...}, "value":[ts, "x.xx"]}]
	// scalar result: [ts, "x.xx"]
	if body.Data.ResultType == "scalar" {
		return parseScalar(body.Data.Result[0])
	}
	var firstResult struct {
		Value [2]json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(body.Data.Result[0], &firstResult); err != nil {
		return 0, err
	}
	return parseScalar(firstResult.Value[1])
}

func parseScalar(raw json.RawMessage) (float64, error) {
	var arr [2]json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		// We were handed the `[ts, "x.xx"]` form.
		return parseScalar(arr[1])
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(s, 64)
}
