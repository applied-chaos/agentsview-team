// Package usage reads an agentsview Instance's usage and activity APIs: the Platform's one client for them.
package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Source struct {
	Base          *url.URL
	Prefix, Token string
}

// For is the Source for Instance id behind the supervisor's Target answer.
func For(id string, base *url.URL, token string) Source { return Source{base, "/i/" + id, token} }

// Get calls /api/v1/<path> and decodes the JSON answer.
func (src Source) Get(ctx context.Context, path string, q url.Values, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.Base.String()+src.Prefix+"/api/v1/"+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+src.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: status %d", path, resp.StatusCode)
	}
	return json.Unmarshal(data, out)
}

// Micros reads upstream's cost, an object {microdollars} or a bare number of microdollars.
type Micros int64

func (m *Micros) UnmarshalJSON(b []byte) error {
	var obj struct {
		Microdollars int64 `json:"microdollars"`
	}
	if json.Unmarshal(b, &obj) == nil {
		*m = Micros(obj.Microdollars)
		return nil
	}
	var n int64
	err := json.Unmarshal(b, &n)
	*m = Micros(n)
	return err
}

func (m Micros) USD() float64 { return float64(m) / 1e6 }

type Tokens struct {
	InputTokens         int64 `json:"inputTokens"`
	OutputTokens        int64 `json:"outputTokens"`
	CacheCreationTokens int64 `json:"cacheCreationTokens"`
	CacheReadTokens     int64 `json:"cacheReadTokens"`
}

func (t Tokens) All() int64 {
	return t.InputTokens + t.OutputTokens + t.CacheCreationTokens + t.CacheReadTokens
}

type Summary struct {
	Totals struct {
		Tokens
		TotalCost Micros `json:"totalCost"`
	} `json:"totals"`
	Daily []struct {
		Date string `json:"date"`
		Tokens
		TotalCost Micros `json:"totalCost"`
	} `json:"daily"`
	ModelTotals []struct {
		Model string `json:"model"`
		Tokens
	} `json:"modelTotals"`
	AgentTotals []struct {
		Agent string `json:"agent"`
		Tokens
	} `json:"agentTotals"`
	SessionCounts struct {
		Total   int64            `json:"total"`
		ByAgent map[string]int64 `json:"byAgent"`
	} `json:"sessionCounts"`
}

// Summary reads usage/summary for the UTC days from to to, with extra filters such as machine or model.
func (src Source) Summary(ctx context.Context, from, to string, extra url.Values) (Summary, error) {
	q := url.Values{"from": {from}, "to": {to}, "timezone": {"UTC"}, "session_counts": {"true"}, "group_models": {"false"}}
	for k, v := range extra {
		q[k] = v
	}
	var sum Summary
	return sum, src.Get(ctx, "usage/summary", q, &sum)
}

func (src Source) SessionDays(ctx context.Context, from, to, machine string) (map[string]int64, error) {
	q := url.Values{"from": {from}, "to": {to}, "timezone": {"UTC"}, "granularity": {"day"}}
	if machine != "" {
		q.Set("machine", machine)
	}
	var a struct {
		Series []struct {
			Date     string `json:"date"`
			Sessions int64  `json:"sessions"`
		} `json:"series"`
	}
	if err := src.Get(ctx, "analytics/activity", q, &a); err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, d := range a.Series {
		out[d.Date] += d.Sessions
	}
	return out, nil
}
