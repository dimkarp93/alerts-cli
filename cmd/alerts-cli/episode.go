package main

import (
	"sort"
	"strings"
	"time"
)

type episode struct {
	labels   labelSet
	start    time.Time
	end      time.Time
	samples  int
	silences []silence
}

func (e episode) name() string     { return e.labels["alertname"] }
func (e episode) group() string    { return e.labels["alertgroup"] }
func (e episode) severity() string { return e.labels["severity"] }
func (e episode) muted() bool      { return len(e.silences) > 0 }

func (e episode) duration() time.Duration {
	d := e.end.Sub(e.start)
	if d < 0 {
		return 0
	}
	return d
}

func (e episode) domain() string {
	if strings.HasPrefix(e.group(), "mvpay_") {
		return "product"
	}
	return "infrastructure"
}

func (e episode) groupKey(mode string) string {
	switch mode {
	case "group":
		return orDash(e.group())
	case "component":
		return orDash(e.labels["component"])
	case "team":
		return orDash(e.labels["team"])
	case "severity":
		return orDash(e.severity())
	case "domain":
		return e.domain()
	default:
		return orDash(e.group())
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

var hiddenLabels = map[string]bool{
	"alertname":   true,
	"alertgroup":  true,
	"severity":    true,
	"team":        true,
	"component":   true,
	"environment": true,
	"oncall":      true,
}

func (e episode) detail() string {
	keys := make([]string, 0, len(e.labels))
	for k := range e.labels {
		if hiddenLabels[k] {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+e.labels[k])
	}
	return strings.Join(parts, " ")
}

func buildEpisodes(all []series, step time.Duration) []episode {
	gap := 3 * step
	var out []episode
	for _, s := range all {
		times := s.times
		sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
		start := times[0]
		prev := times[0]
		count := 1
		flush := func() {
			out = append(out, episode{
				labels:  s.labels,
				start:   start,
				end:     prev.Add(step),
				samples: count,
			})
		}
		for _, t := range times[1:] {
			if t.Sub(prev) > gap {
				flush()
				start = t
				count = 0
			}
			prev = t
			count++
		}
		flush()
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].start.Equal(out[j].start) {
			return out[i].start.Before(out[j].start)
		}
		if out[i].name() != out[j].name() {
			return out[i].name() < out[j].name()
		}
		return out[i].detail() < out[j].detail()
	})
	return out
}

func annotateSilences(eps []episode, silences []silence) {
	for i := range eps {
		for _, s := range silences {
			if s.overlaps(eps[i].start, eps[i].end) && s.matches(eps[i].labels) {
				eps[i].silences = append(eps[i].silences, s)
			}
		}
	}
}
