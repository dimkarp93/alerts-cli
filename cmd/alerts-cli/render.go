package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

type reporter struct {
	out           io.Writer
	loc           *time.Location
	opts          options
	days          []time.Time
	silencesKnown bool
}

func (r *reporter) report(eps []episode) error {
	if r.opts.format == "json" {
		return r.jsonReport(eps)
	}
	r.header(eps)
	if len(eps) == 0 {
		fmt.Fprintln(r.out, "ничего не сработало под заданными фильтрами")
		return nil
	}
	switch r.opts.format {
	case "chrono":
		r.chrono(eps)
	case "by-name":
		r.byName(eps)
	case "by-group":
		r.byGroup(eps)
	case "stats":
		r.stats(eps)
	}
	return nil
}

func (r *reporter) header(eps []episode) {
	period := periodLabel(r.days)
	muted := 0
	for _, e := range eps {
		if e.muted() {
			muted++
		}
	}
	filters := []string{"mode=" + r.opts.mode, "state=" + r.opts.state}
	if r.opts.severity != "" {
		filters = append(filters, "severity="+r.opts.severity)
	}
	if r.opts.env != "" {
		filters = append(filters, "env="+r.opts.env)
	}
	if r.opts.selector != "" {
		filters = append(filters, "selector="+r.opts.selector)
	}
	fmt.Fprintf(r.out, "период: %s (%s)\n", period, r.opts.tz)
	fmt.Fprintf(r.out, "фильтры: %s\n", strings.Join(filters, ", "))
	fmt.Fprintf(r.out, "срабатываний: %d", len(eps))
	if r.silencesKnown {
		fmt.Fprintf(r.out, " (замьючено: %d, не замьючено: %d)", muted, len(eps)-muted)
	} else {
		fmt.Fprintf(r.out, " (статус мьюта неизвестен)")
	}
	fmt.Fprint(r.out, "\n\n")
}

var alertCols = []column{
	col("ВРЕМЯ"),
	col("УР."),
	col("АЛЕРТ"),
	col("ГРУППА"),
	rcol("ДЛИТ."),
	col("МЬЮТ С"),
	col("МЬЮТ ДО"),
	col("КЕМ"),
	col("КОММЕНТАРИЙ"),
	col("МЕТКИ"),
}

func (r *reporter) timeLayout() string {
	if len(r.days) > 1 {
		return "01-02 15:04:05"
	}
	return "15:04:05"
}

func (r *reporter) table(indent string, hidden ...string) *table {
	t := newTable(indent, alertCols...)
	t.hide(hidden...)
	if !r.opts.showLabels {
		t.hide("МЕТКИ")
	}
	return t
}

func (r *reporter) row(e episode) []string {
	var from, to, by, comment string
	if e.muted() {
		s := e.silences[0]
		from = s.StartsAt.In(r.loc).Format("01-02 15:04")
		to = s.EndsAt.In(r.loc).Format("01-02 15:04")
		by = orDash(s.CreatedBy)
		if len(e.silences) > 1 {
			by += fmt.Sprintf(" +%d", len(e.silences)-1)
		}
		comment = clip(flatten(s.Comment), r.opts.colWidth)
	}
	labels := ""
	if r.opts.showLabels {
		labels = clip(e.detail(), r.opts.colWidth)
	}
	return []string{
		e.start.In(r.loc).Format(r.timeLayout()),
		sevTag(e.severity()),
		e.name(),
		orDash(e.group()),
		dur(e.duration()),
		from,
		to,
		by,
		comment,
		labels,
	}
}

func (r *reporter) chrono(eps []episode) {
	t := r.table("")
	for _, e := range eps {
		t.add(r.row(e)...)
	}
	t.render(r.out)
}

func (r *reporter) byName(eps []episode) {
	names := map[string][]episode{}
	for _, e := range eps {
		names[e.name()] = append(names[e.name()], e)
	}
	for _, name := range sortedByCount(names) {
		list := names[name]
		section(r.out, "%s  %s  [%s] — %d срабат., суммарно %s, замьючено %d",
			sevTag(list[0].severity()), name, orDash(list[0].group()), len(list), dur(totalDur(list)), countMuted(list))
		t := r.table("  ", "АЛЕРТ", "ГРУППА", "УР.")
		for _, e := range list {
			t.add(r.row(e)...)
		}
		t.render(r.out)
		fmt.Fprintln(r.out)
	}
}

func (r *reporter) byGroup(eps []episode) {
	groups := map[string][]episode{}
	for _, e := range eps {
		groups[e.groupKey(r.opts.groupBy)] = append(groups[e.groupKey(r.opts.groupBy)], e)
	}
	for _, g := range sortedByCount(groups) {
		list := groups[g]
		names := map[string][]episode{}
		for _, e := range list {
			names[e.name()] = append(names[e.name()], e)
		}
		section(r.out, "%s — %d срабат., %d алерт(ов), суммарно %s, замьючено %d",
			g, len(list), len(names), dur(totalDur(list)), countMuted(list))
		t := r.table("  ", "ГРУППА")
		for _, name := range sortedByCount(names) {
			for _, e := range names[name] {
				t.add(r.row(e)...)
			}
		}
		t.render(r.out)
		fmt.Fprintln(r.out)
	}
}

func (r *reporter) stats(eps []episode) {
	byName := map[string][]episode{}
	byGroup := map[string][]episode{}
	for _, e := range eps {
		byName[e.name()] = append(byName[e.name()], e)
		byGroup[e.groupKey(r.opts.groupBy)] = append(byGroup[e.groupKey(r.opts.groupBy)], e)
	}
	multiDay := len(r.days) > 1

	section(r.out, "по алертам")
	t := newTable("", rcol("КОЛ-ВО"), col("УР."), col("АЛЕРТ"), col("ГРУППА"), rcol("СУММАРНО"), rcol("ЗАМЬЮЧ."), col("ПО ДНЯМ"))
	for _, name := range sortedByCount(byName) {
		list := byName[name]
		days := ""
		if multiDay {
			days = perDay(list, r.loc)
		}
		t.add(fmt.Sprint(len(list)), sevTag(list[0].severity()), name, orDash(list[0].group()),
			dur(totalDur(list)), fmt.Sprint(countMuted(list)), days)
	}
	t.render(r.out)

	fmt.Fprintln(r.out)
	section(r.out, "по группам (%s)", r.opts.groupBy)
	t = newTable("", rcol("КОЛ-ВО"), col("ГРУППА"), rcol("АЛЕРТОВ"), rcol("СУММАРНО"), rcol("ЗАМЬЮЧ."), col("ПО ДНЯМ"))
	for _, g := range sortedByCount(byGroup) {
		list := byGroup[g]
		uniq := map[string]bool{}
		for _, e := range list {
			uniq[e.name()] = true
		}
		days := ""
		if multiDay {
			days = perDay(list, r.loc)
		}
		t.add(fmt.Sprint(len(list)), g, fmt.Sprint(len(uniq)), dur(totalDur(list)), fmt.Sprint(countMuted(list)), days)
	}
	t.render(r.out)
}

func sortedByCount(m map[string][]episode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(m[keys[i]]) != len(m[keys[j]]) {
			return len(m[keys[i]]) > len(m[keys[j]])
		}
		return keys[i] < keys[j]
	})
	return keys
}

func totalDur(eps []episode) time.Duration {
	var total time.Duration
	for _, e := range eps {
		total += e.duration()
	}
	return total
}

func countMuted(eps []episode) int {
	n := 0
	for _, e := range eps {
		if e.muted() {
			n++
		}
	}
	return n
}

func perDay(eps []episode, loc *time.Location) string {
	counts := map[string]int{}
	for _, e := range eps {
		counts[e.start.In(loc).Format("01-02")]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, counts[k]))
	}
	return strings.Join(parts, " ")
}

func sevTag(s string) string {
	switch s {
	case "critical":
		return "CRIT"
	case "warning":
		return "WARN"
	case "":
		return "-"
	default:
		return strings.ToUpper(s)
	}
}

func dur(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}
