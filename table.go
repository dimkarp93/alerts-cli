package main

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type column struct {
	title string
	right bool
}

func col(title string) column  { return column{title: title} }
func rcol(title string) column { return column{title: title, right: true} }

type table struct {
	cols   []column
	rows   [][]string
	indent string
	skip   map[string]bool
}

func newTable(indent string, cols ...column) *table {
	return &table{cols: cols, indent: indent, skip: map[string]bool{}}
}

func (t *table) hide(titles ...string) {
	for _, s := range titles {
		t.skip[s] = true
	}
}

func (t *table) add(cells ...string) {
	t.rows = append(t.rows, cells)
}

func (t *table) render(w io.Writer) {
	if len(t.rows) == 0 {
		return
	}
	keep := make([]int, 0, len(t.cols))
	for i, c := range t.cols {
		if t.skip[c.title] {
			continue
		}
		for _, r := range t.rows {
			if i < len(r) && r[i] != "" {
				keep = append(keep, i)
				break
			}
		}
	}
	if len(keep) == 0 {
		return
	}
	widths := make([]int, len(keep))
	for j, i := range keep {
		widths[j] = runeLen(t.cols[i].title)
		for _, r := range t.rows {
			if i < len(r) {
				if l := runeLen(r[i]); l > widths[j] {
					widths[j] = l
				}
			}
		}
	}
	rule := func(left, mid, right string) {
		parts := make([]string, len(keep))
		for j := range keep {
			parts[j] = strings.Repeat("─", widths[j]+2)
		}
		fmt.Fprintln(w, t.indent+left+strings.Join(parts, mid)+right)
	}
	row := func(cells []string) {
		parts := make([]string, len(keep))
		for j, i := range keep {
			v := ""
			if i < len(cells) {
				v = cells[i]
			}
			parts[j] = " " + pad(v, widths[j], t.cols[i].right) + " "
		}
		fmt.Fprintln(w, t.indent+"│"+strings.Join(parts, "│")+"│")
	}
	head := make([]string, len(t.cols))
	for i, c := range t.cols {
		head[i] = c.title
	}
	rule("┌", "┬", "┐")
	row(head)
	rule("├", "┼", "┤")
	for _, r := range t.rows {
		row(r)
	}
	rule("└", "┴", "┘")
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func pad(s string, width int, right bool) string {
	gap := strings.Repeat(" ", width-runeLen(s))
	if right {
		return gap + s
	}
	return s + gap
}

func clip(s string, limit int) string {
	if limit <= 0 || runeLen(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit-1]) + "…"
}

func section(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "▌ "+format+"\n", args...)
}
