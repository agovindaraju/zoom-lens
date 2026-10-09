// Package budget implements the series-budget guardrail: before escalating,
// ask Prometheus how many head series it holds and refuse if over the limit.
package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Checker queries Prometheus for its head series count.
type Checker struct {
	baseURL   string
	maxSeries float64
	client    *http.Client
}

// Result is the outcome of a budget check.
type Result struct {
	Allowed bool
	Series  float64
	Limit   float64
}

// DefaultQuery is the PromQL used to measure current load.
const DefaultQuery = "sum(prometheus_tsdb_head_series)"

// New returns a Checker. maxSeries <= 0 disables the check.
func New(prometheusURL string, maxSeries float64) *Checker {
	return &Checker{baseURL: prometheusURL, maxSeries: maxSeries, client: &http.Client{Timeout: 5 * time.Second}}
}

// Check reports whether a further escalation fits in the budget. On a query
// error it fails closed (Allowed=false) so a broken Prometheus cannot cause an
// unbounded expansion.
func (c *Checker) Check(ctx context.Context) (Result, error) {
	if c == nil || c.maxSeries <= 0 {
		return Result{Allowed: true}, nil
	}
	v, err := c.query(ctx, DefaultQuery)
	if err != nil {
		return Result{Allowed: false, Limit: c.maxSeries}, err
	}
	return Result{Allowed: v < c.maxSeries, Series: v, Limit: c.maxSeries}, nil
}

type promResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value [2]any `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (c *Checker) query(ctx context.Context, q string) (float64, error) {
	u := c.baseURL + "/api/v1/query?" + url.Values{"query": {q}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("query prometheus: %w", err)
	}
	defer resp.Body.Close()
	var pr promResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return 0, fmt.Errorf("decode prometheus response (HTTP %d): %w", resp.StatusCode, err)
	}
	if pr.Status != "success" {
		return 0, fmt.Errorf("prometheus query failed: %s", pr.Error)
	}
	if len(pr.Data.Result) == 0 {
		return 0, fmt.Errorf("prometheus query %q returned no data", q)
	}
	s, ok := pr.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected prometheus value %v", pr.Data.Result[0].Value[1])
	}
	return strconv.ParseFloat(s, 64)
}
