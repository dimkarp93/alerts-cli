package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const dayLayout = "2006-01-02"

func main() {
	if build().Handle(os.Args[1:]) {
		return
	}
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "alerts":
		err = runAlerts(args)
	case "mutes":
		err = runMutes(args)
	case "help", "-h", "--help":
		usage()
		return
	default:
		usage()
		fmt.Fprintf(os.Stderr, "\nalerts-cli: unknown command %q\n", cmd)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "alerts-cli:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `alerts-cli - что реально сработало и что было замьючено, по дням.

Usage:
  alerts-cli alerts [flags]   срабатывания алертов (история vmalert в VictoriaMetrics)
  alerts-cli mutes  [flags]   включения мьютов (сайленсы Alertmanager)

  alerts-cli <command> -h     флаги подкоманды
  alerts-cli --version        версия (-v)
  alerts-cli --origin         репозиторий, из которого собран бинарь
  alerts-cli --buildinfo      полная информация о сборке

Examples:
  alerts-cli alerts -date yesterday -format stats
  alerts-cli alerts -date 2026-09-10-2026-09-14 -mode unmuted -severity critical
  alerts-cli mutes -date 2026-09-10-2026-09-15
  alerts-cli mutes -date 2026-09-15 -format by-group -group-by creator
`)
}

func splitSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out[p] = true
		}
	}
	return out
}

type commonFlags struct {
	vmURL    string
	amURL    string
	dates    string
	severity string
	format   string
	groupBy  string
	tz       string
	colWidth int
	timeout  time.Duration
	compact  bool
}

func (c *commonFlags) bind(fs *flag.FlagSet, defFormat, defGroupBy, formats, groupBys string) {
	fs.StringVar(&c.dates, "date", "today", "day, list (2026-09-10,2026-09-12) or range (2026-09-10-2026-09-15); also today/yesterday/-N")
	fs.StringVar(&c.severity, "severity", "", "comma-separated severity filter (warning,critical,...); empty = all")
	fs.StringVar(&c.format, "format", defFormat, "output format: "+formats)
	fs.StringVar(&c.groupBy, "group-by", defGroupBy, "grouping key for by-group/stats: "+groupBys)
	fs.StringVar(&c.tz, "tz", "Europe/Moscow", "timezone for day boundaries and output")
	fs.IntVar(&c.colWidth, "col-width", 48, "max width of comment/labels columns in table output; 0 = unlimited")
	fs.DurationVar(&c.timeout, "timeout", 60*time.Second, "HTTP timeout")
	fs.BoolVar(&c.compact, "json-compact", false, "with -format json: single-line JSON instead of indented")
}

func (c *commonFlags) resolve(formats, groupBys []string) (*time.Location, []time.Time, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, nil, err
	}
	c.vmURL = cfg.VM
	c.amURL = cfg.AM
	loc, err := time.LoadLocation(c.tz)
	if err != nil {
		return nil, nil, fmt.Errorf("timezone %q: %w", c.tz, err)
	}
	if !contains(formats, c.format) {
		return nil, nil, fmt.Errorf("unknown -format %q (want %s)", c.format, strings.Join(formats, "|"))
	}
	if !contains(groupBys, c.groupBy) {
		return nil, nil, fmt.Errorf("unknown -group-by %q (want %s)", c.groupBy, strings.Join(groupBys, "|"))
	}
	days, err := parseDates(c.dates, loc, time.Now().In(loc))
	if err != nil {
		return nil, nil, err
	}
	return loc, days, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

var rangeRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(\d{4}-\d{2}-\d{2})$`)

func parseDates(spec string, loc *time.Location, now time.Time) ([]time.Time, error) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	seen := map[string]bool{}
	var days []time.Time
	add := func(d time.Time) {
		k := d.Format(dayLayout)
		if !seen[k] {
			seen[k] = true
			days = append(days, d)
		}
	}
	for _, token := range strings.Split(spec, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		switch {
		case token == "today":
			add(today)
		case token == "yesterday":
			add(today.AddDate(0, 0, -1))
		case strings.HasPrefix(token, "-") && !rangeRe.MatchString(token):
			n, err := strconv.Atoi(token)
			if err != nil {
				return nil, fmt.Errorf("bad date %q", token)
			}
			add(today.AddDate(0, 0, n))
		case rangeRe.MatchString(token):
			p := rangeRe.FindStringSubmatch(token)
			from, err := time.ParseInLocation(dayLayout, p[1], loc)
			if err != nil {
				return nil, err
			}
			to, err := time.ParseInLocation(dayLayout, p[2], loc)
			if err != nil {
				return nil, err
			}
			if to.Before(from) {
				from, to = to, from
			}
			for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
				add(d)
			}
		default:
			d, err := time.ParseInLocation(dayLayout, token, loc)
			if err != nil {
				return nil, fmt.Errorf("bad date %q (want YYYY-MM-DD, a range or today/yesterday/-N)", token)
			}
			add(d)
		}
	}
	if len(days) == 0 {
		return nil, fmt.Errorf("no days selected")
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	return days, nil
}

func contiguous(days []time.Time) [][]time.Time {
	var out [][]time.Time
	cur := []time.Time{days[0]}
	for _, d := range days[1:] {
		if d.Equal(cur[len(cur)-1].AddDate(0, 0, 1)) {
			cur = append(cur, d)
			continue
		}
		out = append(out, cur)
		cur = []time.Time{d}
	}
	return append(out, cur)
}

func periodLabel(days []time.Time) string {
	if len(days) == 1 {
		return days[0].Format(dayLayout)
	}
	span := int(days[len(days)-1].Sub(days[0]).Hours()/24) + 1
	if span == len(days) {
		return fmt.Sprintf("%s .. %s (%d дн.)", days[0].Format(dayLayout), days[len(days)-1].Format(dayLayout), len(days))
	}
	parts := make([]string, 0, len(days))
	for _, d := range days {
		parts = append(parts, d.Format(dayLayout))
	}
	return strings.Join(parts, ", ")
}
