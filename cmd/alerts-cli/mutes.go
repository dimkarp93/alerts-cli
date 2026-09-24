package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type muteOptions struct {
	commonFlags
	filter       string
	creator      string
	showMatchers bool
}

type mute struct {
	sil        silence
	alerts     []string
	groups     []string
	severities []string
}

var muteFormats = []string{"chrono", "by-name", "by-group", "json"}
var muteFilters = []string{"all", "created", "expired"}
var muteGroupBys = []string{"group", "severity", "creator", "domain"}

func runMutes(args []string) error {
	var o muteOptions
	fs := flag.NewFlagSet("mutes", flag.ExitOnError)
	o.bind(fs, "chrono", "group", strings.Join(muteFormats, "|"), strings.Join(muteGroupBys, "|"))
	fs.StringVar(&o.filter, "filter", "all", "which mutes to show: all (active during the days)|created (switched on then)|expired (ended then)")
	fs.StringVar(&o.creator, "by", "", "filter by author (substring, case-insensitive)")
	fs.BoolVar(&o.showMatchers, "matchers", true, "print the silence matchers")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `alerts-cli mutes - когда и кем включались мьюты (сайленсы Alertmanager).

Alertmanager хранит истёкшие сайленсы ограниченное время, поэтому давние мьюты уже недоступны.

Examples:
  alerts-cli mutes -date 2026-09-10-2026-09-15
  alerts-cli mutes -date 2026-09-15 -filter created
  alerts-cli mutes -date 2026-09-15 -filter expired -severity critical
  alerts-cli mutes -date 2026-09-10-2026-09-15 -format by-name
  alerts-cli mutes -date 2026-09-10-2026-09-15 -format by-group -group-by creator
  alerts-cli mutes -date 2026-09-15 -format json

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	loc, days, err := o.resolve(muteFormats, muteGroupBys)
	if err != nil {
		return err
	}
	if !contains(muteFilters, o.filter) {
		return fmt.Errorf("unknown -filter %q (want %s)", o.filter, strings.Join(muteFilters, "|"))
	}

	client := &http.Client{Timeout: o.timeout}
	sil, err := fetchSilences(client, o.amURL)
	if err != nil {
		return err
	}

	meta, err := fetchAlertSeries(client, o.vmURL, days[0].AddDate(0, 0, -30), time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "alerts-cli: alert metadata unavailable (%v), severity/group may be unknown\n", err)
	}

	mutes := make([]mute, 0, len(sil))
	for _, s := range sil {
		mutes = append(mutes, newMute(s, meta))
	}
	mutes = filterMutes(mutes, days, o)
	sort.Slice(mutes, func(i, j int) bool {
		if !mutes[i].sil.StartsAt.Equal(mutes[j].sil.StartsAt) {
			return mutes[i].sil.StartsAt.Before(mutes[j].sil.StartsAt)
		}
		return mutes[i].alert() < mutes[j].alert()
	})

	m := &muteReporter{out: os.Stdout, loc: loc, opts: o, days: days}
	return m.report(mutes)
}

func newMute(s silence, meta []labelSet) mute {
	m := mute{sil: s}
	explicit := map[string][]string{}
	for _, mt := range s.Matchers {
		if mt.IsRegex || (mt.IsEqual != nil && !*mt.IsEqual) {
			continue
		}
		explicit[mt.Name] = append(explicit[mt.Name], mt.Value)
	}
	derived := map[string]map[string]bool{
		"alertname":  {},
		"alertgroup": {},
		"severity":   {},
	}
	for _, ls := range meta {
		if !s.matches(ls) {
			continue
		}
		for k := range derived {
			if v := ls[k]; v != "" {
				derived[k][v] = true
			}
		}
	}
	pick := func(name string) []string {
		if v, ok := explicit[name]; ok {
			return v
		}
		return sortedKeys(derived[name])
	}
	m.alerts = pick("alertname")
	m.groups = pick("alertgroup")
	m.severities = pick("severity")
	return m
}

func flatten(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func joinOrDash(v []string) string {
	if len(v) == 0 {
		return "-"
	}
	return strings.Join(v, ",")
}

func (m mute) alert() string    { return joinOrDash(m.alerts) }
func (m mute) group() string    { return joinOrDash(m.groups) }
func (m mute) severity() string { return joinOrDash(m.severities) }

func (m mute) domain() string {
	if len(m.groups) == 0 {
		return "-"
	}
	product, infra := false, false
	for _, g := range m.groups {
		if strings.HasPrefix(g, "mvpay_") {
			product = true
		} else {
			infra = true
		}
	}
	switch {
	case product && infra:
		return "mixed"
	case product:
		return "product"
	default:
		return "infrastructure"
	}
}

func (m mute) groupKey(mode string) string {
	switch mode {
	case "severity":
		return m.severity()
	case "creator":
		return m.creator()
	case "domain":
		return m.domain()
	default:
		return m.group()
	}
}

func (m mute) creator() string {
	if c := strings.TrimSpace(m.sil.CreatedBy); c != "" {
		return c
	}
	return "?"
}

func (m mute) duration() time.Duration { return m.sil.EndsAt.Sub(m.sil.StartsAt) }

func (m mute) matchersText() string {
	parts := make([]string, 0, len(m.sil.Matchers))
	for _, mt := range m.sil.Matchers {
		if mt.Name == "alertname" || mt.Name == "alertgroup" || mt.Name == "severity" {
			continue
		}
		op := "="
		switch {
		case mt.IsRegex && mt.IsEqual != nil && !*mt.IsEqual:
			op = "!~"
		case mt.IsRegex:
			op = "=~"
		case mt.IsEqual != nil && !*mt.IsEqual:
			op = "!="
		}
		parts = append(parts, mt.Name+op+mt.Value)
	}
	return strings.Join(parts, " ")
}

func filterMutes(mutes []mute, days []time.Time, o muteOptions) []mute {
	sev := splitSet(o.severity)
	creator := strings.ToLower(strings.TrimSpace(o.creator))
	kept := mutes[:0]
	for _, m := range mutes {
		if !inDays(m, days, o.filter) {
			continue
		}
		if len(sev) > 0 {
			hit := false
			for _, s := range m.severities {
				if sev[s] {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		if creator != "" && !strings.Contains(strings.ToLower(m.creator()), creator) {
			continue
		}
		kept = append(kept, m)
	}
	return kept
}

func inDays(m mute, days []time.Time, filter string) bool {
	for _, d := range days {
		next := d.AddDate(0, 0, 1)
		switch filter {
		case "created":
			if !m.sil.StartsAt.Before(d) && m.sil.StartsAt.Before(next) {
				return true
			}
		case "expired":
			if !m.sil.EndsAt.After(time.Now()) && !m.sil.EndsAt.Before(d) && m.sil.EndsAt.Before(next) {
				return true
			}
		default:
			if m.sil.overlaps(d, next) {
				return true
			}
		}
	}
	return false
}

type muteReporter struct {
	out  *os.File
	loc  *time.Location
	opts muteOptions
	days []time.Time
}

func (r *muteReporter) report(mutes []mute) error {
	if r.opts.format == "json" {
		return r.jsonReport(mutes)
	}
	r.header(mutes)
	if len(mutes) == 0 {
		fmt.Fprintln(r.out, "мьютов под заданными фильтрами нет")
		return nil
	}
	switch r.opts.format {
	case "chrono":
		t := r.table("")
		for _, m := range mutes {
			t.add(r.row(m)...)
		}
		t.render(r.out)
	case "by-name":
		r.grouped(mutes, func(m mute) string { return m.alert() }, "АЛЕРТ")
	case "by-group":
		hidden := "ГРУППА"
		switch r.opts.groupBy {
		case "severity":
			hidden = "УР."
		case "creator":
			hidden = "КЕМ"
		case "domain":
			hidden = ""
		}
		r.grouped(mutes, func(m mute) string { return m.groupKey(r.opts.groupBy) }, hidden)
	}
	return nil
}

func (r *muteReporter) header(mutes []mute) {
	active := 0
	authors := map[string]bool{}
	for _, m := range mutes {
		if m.sil.Status.State == "active" {
			active++
		}
		authors[m.creator()] = true
	}
	filters := []string{}
	switch r.opts.filter {
	case "created":
		filters = append(filters, "filter=created (включены в эти дни)")
	case "expired":
		filters = append(filters, "filter=expired (истекли в эти дни)")
	default:
		filters = append(filters, "filter=all (действовали в эти дни)")
	}
	if r.opts.severity != "" {
		filters = append(filters, "severity="+r.opts.severity)
	}
	if r.opts.creator != "" {
		filters = append(filters, "by="+r.opts.creator)
	}
	fmt.Fprintf(r.out, "период: %s (%s)\n", periodLabel(r.days), r.opts.tz)
	fmt.Fprintf(r.out, "фильтры: %s\n", strings.Join(filters, ", "))
	fmt.Fprintf(r.out, "мьютов: %d (сейчас активны: %d, авторов: %d)\n\n", len(mutes), active, len(authors))
}

var muteCols = []column{
	col("ВКЛЮЧЁН"),
	col("ДО"),
	rcol("ДЛИТ."),
	col("СТАТУС"),
	col("УР."),
	col("АЛЕРТ"),
	col("ГРУППА"),
	col("КЕМ"),
	col("КОММЕНТАРИЙ"),
	col("МАТЧЕРЫ"),
}

func (r *muteReporter) table(indent string, hidden ...string) *table {
	t := newTable(indent, muteCols...)
	t.hide(hidden...)
	if !r.opts.showMatchers {
		t.hide("МАТЧЕРЫ")
	}
	return t
}

func (r *muteReporter) row(m mute) []string {
	layout := "01-02 15:04:05"
	if len(r.days) == 1 && r.opts.filter == "created" {
		layout = "15:04:05"
	}
	matchers := ""
	if r.opts.showMatchers {
		matchers = clip(m.matchersText(), r.opts.colWidth)
	}
	return []string{
		m.sil.StartsAt.In(r.loc).Format(layout),
		m.sil.EndsAt.In(r.loc).Format("01-02 15:04"),
		dur(m.duration()),
		m.sil.Status.State,
		sevTag(m.severity()),
		m.alert(),
		m.group(),
		m.creator(),
		clip(flatten(m.sil.Comment), r.opts.colWidth),
		matchers,
	}
}

func (r *muteReporter) grouped(mutes []mute, key func(mute) string, hidden string) {
	groups := map[string][]mute{}
	for _, m := range mutes {
		groups[key(m)] = append(groups[key(m)], m)
	}
	names := make([]string, 0, len(groups))
	for k := range groups {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if len(groups[names[i]]) != len(groups[names[j]]) {
			return len(groups[names[i]]) > len(groups[names[j]])
		}
		return names[i] < names[j]
	})
	for _, n := range names {
		list := groups[n]
		authors := map[string]bool{}
		active := 0
		for _, m := range list {
			authors[m.creator()] = true
			if m.sil.Status.State == "active" {
				active++
			}
		}
		section(r.out, "%s — %d мьют(ов), активны: %d, авторы: %s", n, len(list), active, strings.Join(sortedKeys(authors), ", "))
		t := r.table("  ", hidden)
		for _, m := range list {
			t.add(r.row(m)...)
		}
		t.render(r.out)
		fmt.Fprintln(r.out)
	}
}

type jsonMute struct {
	ID         string    `json:"id"`
	CreatedBy  string    `json:"created_by"`
	Comment    string    `json:"comment"`
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
	Day        string    `json:"day"`
	DurationS  int       `json:"duration_seconds"`
	State      string    `json:"state"`
	Alerts     []string  `json:"alerts"`
	Groups     []string  `json:"groups"`
	Severities []string  `json:"severities"`
	Domain     string    `json:"domain"`
	Matchers   []string  `json:"matchers"`
}

type jsonMuteReport struct {
	Days     []string   `json:"days"`
	Timezone string     `json:"timezone"`
	Severity string     `json:"severity_filter,omitempty"`
	Creator  string     `json:"creator_filter,omitempty"`
	Filter   string     `json:"filter"`
	Total    int        `json:"total"`
	Mutes    []jsonMute `json:"mutes"`
}

func (r *muteReporter) jsonReport(mutes []mute) error {
	rep := jsonMuteReport{
		Timezone: r.opts.tz,
		Severity: r.opts.severity,
		Creator:  r.opts.creator,
		Filter:   r.opts.filter,
		Total:    len(mutes),
		Mutes:    make([]jsonMute, 0, len(mutes)),
	}
	for _, d := range r.days {
		rep.Days = append(rep.Days, d.Format(dayLayout))
	}
	for _, m := range mutes {
		start := m.sil.StartsAt.In(r.loc)
		matchers := make([]string, 0, len(m.sil.Matchers))
		for _, mt := range m.sil.Matchers {
			op := "="
			switch {
			case mt.IsRegex && mt.IsEqual != nil && !*mt.IsEqual:
				op = "!~"
			case mt.IsRegex:
				op = "=~"
			case mt.IsEqual != nil && !*mt.IsEqual:
				op = "!="
			}
			matchers = append(matchers, mt.Name+op+mt.Value)
		}
		rep.Mutes = append(rep.Mutes, jsonMute{
			ID:         m.sil.ID,
			CreatedBy:  m.creator(),
			Comment:    m.sil.Comment,
			StartsAt:   start,
			EndsAt:     m.sil.EndsAt.In(r.loc),
			Day:        start.Format(dayLayout),
			DurationS:  int(m.duration().Round(time.Second).Seconds()),
			State:      m.sil.Status.State,
			Alerts:     m.alerts,
			Groups:     m.groups,
			Severities: m.severities,
			Domain:     m.domain(),
			Matchers:   matchers,
		})
	}
	enc := json.NewEncoder(r.out)
	if !r.opts.compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(rep)
}
