package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

func getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// PromQuery runs an instant query and renders each series as "labels => value".
func PromQuery(ctx context.Context, base, query string) (string, error) {
	var r struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  [2]any            `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := getJSON(ctx, base+"/api/v1/query?query="+url.QueryEscape(query), &r); err != nil {
		return "", err
	}
	if len(r.Data.Result) == 0 {
		return "(no data)", nil
	}
	var b strings.Builder
	for _, s := range r.Data.Result {
		fmt.Fprintf(&b, "%v => %v\n", s.Metric, s.Value[1])
	}
	return b.String(), nil
}

// LokiQuery returns up to limit log lines for a LogQL query over the last `since`.
func LokiQuery(ctx context.Context, base, logql string, since time.Duration, limit int) (string, error) {
	q := url.Values{}
	q.Set("query", logql)
	q.Set("since", since.String())
	q.Set("limit", fmt.Sprint(limit))
	var r struct {
		Data struct {
			Result []struct {
				Values [][2]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := getJSON(ctx, base+"/loki/api/v1/query_range?"+q.Encode(), &r); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, s := range r.Data.Result {
		for _, v := range s.Values {
			b.WriteString(v[1])
			b.WriteByte('\n')
		}
	}
	if b.Len() == 0 {
		return "(no matching logs)", nil
	}
	return b.String(), nil
}
