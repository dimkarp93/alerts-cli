package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type options struct {
	commonFlags
	mode       string
	env        string
	selector   string
	state      string
	step       time.Duration
	noSilences bool
	showLabels bool
}

var alertFormats = []string{"chrono", "by-name", "by-group", "stats", "json"}
var alertGroupBys = []string{"group", "component", "team", "domain", "severity"}

func runAlerts(args []string) error {
	var o options
	fs := flag.NewFlagSet("alerts", flag.ExitOnError)
	o.bind(fs, "chrono", "group", strings.Join(alertFormats, "|"), strings.Join(alertGroupBys, "|"))
	fs.StringVar(&o.mode, "mode", "all", "which alerts to show: all|unmuted|muted")
	fs.StringVar(&o.env, "env", "prod", "environment label filter; empty = all")
	fs.StringVar(&o.selector, "selector", "", "extra PromQL label matchers, e.g. alertname=~\"Payments.*\",team=\"mvpay\"")
	fs.StringVar(&o.state, "state", "firing", "alert state: firing|pending|any")
	fs.DurationVar(&o.step, "step", time.Minute, "query resolution; also the gap used to split firing episodes")
	fs.BoolVar(&o.noSilences, "no-silences", false, "skip Alertmanager lookup (everything counts as unmuted)")
	fs.BoolVar(&o.showLabels, "labels", true, "print distinguishing labels of each firing")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `alerts-cli alerts - срабатывания алертов за выбранные дни.

Examples:
  alerts-cli alerts
  alerts-cli alerts -date yesterday -format stats
  alerts-cli alerts -date 2026-09-10-2026-09-14 -mode unmuted -severity critical
  alerts-cli alerts -date 2026-09-12,2026-09-15 -format by-group -group-by domain
  alerts-cli alerts -format json > alerts.json

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	loc, days, err := o.resolve(alertFormats, alertGroupBys)
	if err != nil {
		return err
	}
	if o.mode != "all" && o.mode != "unmuted" && o.mode != "muted" {
		return fmt.Errorf("unknown -mode %q", o.mode)
	}
	if o.step < 15*time.Second {
		return fmt.Errorf("-step must be at least 15s")
	}

	sevFilter := splitSet(o.severity)
	client := &http.Client{Timeout: o.timeout}
	query := buildQuery(o.state, o.env, o.selector)

	var eps []episode
	for _, chunk := range contiguous(days) {
		from := chunk[0]
		to := chunk[len(chunk)-1].Add(24 * time.Hour)
		if now := time.Now().In(loc); to.After(now) {
			to = now
		}
		if !to.After(from) {
			continue
		}
		s, err := queryRange(client, o.vmURL, query, from, to, o.step)
		if err != nil {
			return err
		}
		eps = append(eps, buildEpisodes(s, o.step)...)
	}

	eps = filterDays(eps, days)
	if len(sevFilter) > 0 {
		kept := eps[:0]
		for _, e := range eps {
			if sevFilter[e.severity()] {
				kept = append(kept, e)
			}
		}
		eps = kept
	}
	sort.SliceStable(eps, func(i, j int) bool { return eps[i].start.Before(eps[j].start) })

	silencesKnown := false
	if !o.noSilences {
		sil, err := fetchSilences(client, o.amURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "alerts-cli: silences unavailable (%v), mute state unknown\n", err)
		} else {
			annotateSilences(eps, sil)
			silencesKnown = true
		}
	}
	if o.mode != "all" {
		if !silencesKnown {
			return fmt.Errorf("-mode %s requires Alertmanager silences", o.mode)
		}
		kept := eps[:0]
		for _, e := range eps {
			if e.muted() == (o.mode == "muted") {
				kept = append(kept, e)
			}
		}
		eps = kept
	}

	r := &reporter{out: os.Stdout, loc: loc, opts: o, days: days, silencesKnown: silencesKnown}
	return r.report(eps)
}

func buildQuery(state, env, selector string) string {
	var m []string
	switch state {
	case "any":
	case "pending":
		m = append(m, `alertstate="pending"`)
	default:
		m = append(m, `alertstate="firing"`)
	}
	if env != "" {
		m = append(m, fmt.Sprintf("environment=%q", env))
	}
	if s := strings.TrimSpace(selector); s != "" {
		m = append(m, s)
	}
	return "ALERTS{" + strings.Join(m, ",") + "}"
}

func filterDays(eps []episode, days []time.Time) []episode {
	kept := eps[:0]
	for _, e := range eps {
		for _, d := range days {
			if e.start.Before(d.AddDate(0, 0, 1)) && e.end.After(d) {
				kept = append(kept, e)
				break
			}
		}
	}
	return kept
}
