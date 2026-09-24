package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type labelSet map[string]string

type series struct {
	labels labelSet
	times  []time.Time
}

type matrixResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Values [][]any           `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func queryRange(client *http.Client, base, query string, start, end time.Time, step time.Duration) ([]series, error) {
	u, err := url.Parse(strings.TrimRight(base, "/") + "/api/v1/query_range")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", query)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", strconv.Itoa(int(step.Seconds()))+"s")
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("victoriametrics %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var parsed matrixResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("victoriametrics error: %s", parsed.Error)
	}

	out := make([]series, 0, len(parsed.Data.Result))
	for _, r := range parsed.Data.Result {
		ls := labelSet{}
		for k, v := range r.Metric {
			if k == "__name__" || k == "alertstate" {
				continue
			}
			ls[k] = v
		}
		s := series{labels: ls, times: make([]time.Time, 0, len(r.Values))}
		for _, v := range r.Values {
			if len(v) == 0 {
				continue
			}
			ts, ok := v[0].(float64)
			if !ok {
				continue
			}
			s.times = append(s.times, time.Unix(int64(ts), 0))
		}
		if len(s.times) > 0 {
			out = append(out, s)
		}
	}
	return out, nil
}

type matcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual *bool  `json:"isEqual"`
	re      *regexp.Regexp
}

type silence struct {
	ID        string    `json:"id"`
	Comment   string    `json:"comment"`
	CreatedBy string    `json:"createdBy"`
	StartsAt  time.Time `json:"startsAt"`
	EndsAt    time.Time `json:"endsAt"`
	Matchers  []matcher `json:"matchers"`
	Status    struct {
		State string `json:"state"`
	} `json:"status"`
}

func fetchSilences(client *http.Client, base string) ([]silence, error) {
	resp, err := client.Get(strings.TrimRight(base, "/") + "/api/v2/silences")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alertmanager %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out []silence
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse silences: %w", err)
	}
	for i := range out {
		for j := range out[i].Matchers {
			m := &out[i].Matchers[j]
			if m.IsRegex {
				re, err := regexp.Compile("^(?:" + m.Value + ")$")
				if err != nil {
					continue
				}
				m.re = re
			}
		}
	}
	return out, nil
}

func (m matcher) matches(ls labelSet) bool {
	v := ls[m.Name]
	var hit bool
	if m.re != nil {
		hit = m.re.MatchString(v)
	} else {
		hit = v == m.Value
	}
	negative := m.IsEqual != nil && !*m.IsEqual
	if negative {
		return !hit
	}
	return hit
}

func (s silence) matches(ls labelSet) bool {
	if len(s.Matchers) == 0 {
		return false
	}
	for _, m := range s.Matchers {
		if !m.matches(ls) {
			return false
		}
	}
	return true
}

func (s silence) overlaps(from, to time.Time) bool {
	return s.StartsAt.Before(to.Add(time.Second)) && s.EndsAt.After(from.Add(-time.Second))
}

type seriesResponse struct {
	Status string              `json:"status"`
	Error  string              `json:"error"`
	Data   []map[string]string `json:"data"`
}

func fetchAlertSeries(client *http.Client, base string, start, end time.Time) ([]labelSet, error) {
	u, err := url.Parse(strings.TrimRight(base, "/") + "/api/v1/series")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("match[]", "ALERTS")
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("victoriametrics %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var parsed seriesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse series: %w", err)
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("victoriametrics error: %s", parsed.Error)
	}
	out := make([]labelSet, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		ls := labelSet{}
		for k, v := range m {
			if k == "__name__" || k == "alertstate" {
				continue
			}
			ls[k] = v
		}
		out = append(out, ls)
	}
	return out, nil
}
