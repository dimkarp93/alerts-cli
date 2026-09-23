package main

import (
	"encoding/json"
	"time"
)

type jsonSilence struct {
	ID        string    `json:"id"`
	CreatedBy string    `json:"created_by"`
	Comment   string    `json:"comment"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
}

type jsonEpisode struct {
	Alert       string            `json:"alert"`
	Group       string            `json:"group"`
	Domain      string            `json:"domain"`
	Component   string            `json:"component,omitempty"`
	Team        string            `json:"team,omitempty"`
	Severity    string            `json:"severity"`
	Start       time.Time         `json:"start"`
	End         time.Time         `json:"end"`
	Day         string            `json:"day"`
	DurationSec int               `json:"duration_seconds"`
	Muted       bool              `json:"muted"`
	Silences    []jsonSilence     `json:"silences,omitempty"`
	Labels      map[string]string `json:"labels"`
}

type jsonCount struct {
	Key         string         `json:"key"`
	Group       string         `json:"group,omitempty"`
	Severity    string         `json:"severity,omitempty"`
	Alerts      int            `json:"distinct_alerts,omitempty"`
	Count       int            `json:"count"`
	Muted       int            `json:"muted"`
	DurationSec int            `json:"total_duration_seconds"`
	PerDay      map[string]int `json:"per_day"`
}

type jsonReport struct {
	Days          []string      `json:"days"`
	Timezone      string        `json:"timezone"`
	Mode          string        `json:"mode"`
	State         string        `json:"state"`
	Severity      string        `json:"severity_filter,omitempty"`
	Environment   string        `json:"environment,omitempty"`
	Selector      string        `json:"selector,omitempty"`
	GroupBy       string        `json:"group_by"`
	SilencesKnown bool          `json:"silences_known"`
	Total         int           `json:"total"`
	TotalMuted    int           `json:"total_muted"`
	Episodes      []jsonEpisode `json:"episodes"`
	ByAlert       []jsonCount   `json:"by_alert"`
	ByGroup       []jsonCount   `json:"by_group"`
}

func (r *reporter) jsonReport(eps []episode) error {
	rep := jsonReport{
		Timezone:      r.opts.tz,
		Mode:          r.opts.mode,
		State:         r.opts.state,
		Severity:      r.opts.severity,
		Environment:   r.opts.env,
		Selector:      r.opts.selector,
		GroupBy:       r.opts.groupBy,
		SilencesKnown: r.silencesKnown,
		Total:         len(eps),
		TotalMuted:    countMuted(eps),
		Episodes:      make([]jsonEpisode, 0, len(eps)),
		ByAlert:       []jsonCount{},
		ByGroup:       []jsonCount{},
	}
	for _, d := range r.days {
		rep.Days = append(rep.Days, d.Format(dayLayout))
	}

	byName := map[string][]episode{}
	byGroup := map[string][]episode{}
	for _, e := range eps {
		start := e.start.In(r.loc)
		je := jsonEpisode{
			Alert:       e.name(),
			Group:       e.group(),
			Domain:      e.domain(),
			Component:   e.labels["component"],
			Team:        e.labels["team"],
			Severity:    e.severity(),
			Start:       start,
			End:         e.end.In(r.loc),
			Day:         start.Format(dayLayout),
			DurationSec: int(e.duration().Round(time.Second).Seconds()),
			Muted:       e.muted(),
			Labels:      e.labels,
		}
		for _, s := range e.silences {
			je.Silences = append(je.Silences, jsonSilence{
				ID:        s.ID,
				CreatedBy: s.CreatedBy,
				Comment:   s.Comment,
				StartsAt:  s.StartsAt.In(r.loc),
				EndsAt:    s.EndsAt.In(r.loc),
			})
		}
		rep.Episodes = append(rep.Episodes, je)
		byName[e.name()] = append(byName[e.name()], e)
		byGroup[e.groupKey(r.opts.groupBy)] = append(byGroup[e.groupKey(r.opts.groupBy)], e)
	}

	for _, name := range sortedByCount(byName) {
		list := byName[name]
		rep.ByAlert = append(rep.ByAlert, jsonCount{
			Key:         name,
			Group:       list[0].group(),
			Severity:    list[0].severity(),
			Count:       len(list),
			Muted:       countMuted(list),
			DurationSec: int(totalDur(list).Round(time.Second).Seconds()),
			PerDay:      perDayMap(list, r.loc),
		})
	}
	for _, g := range sortedByCount(byGroup) {
		list := byGroup[g]
		uniq := map[string]bool{}
		for _, e := range list {
			uniq[e.name()] = true
		}
		rep.ByGroup = append(rep.ByGroup, jsonCount{
			Key:         g,
			Alerts:      len(uniq),
			Count:       len(list),
			Muted:       countMuted(list),
			DurationSec: int(totalDur(list).Round(time.Second).Seconds()),
			PerDay:      perDayMap(list, r.loc),
		})
	}

	enc := json.NewEncoder(r.out)
	if !r.opts.compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(rep)
}

func perDayMap(eps []episode, loc *time.Location) map[string]int {
	out := map[string]int{}
	for _, e := range eps {
		out[e.start.In(loc).Format(dayLayout)]++
	}
	return out
}
