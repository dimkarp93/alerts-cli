package pathreport

import (
	"fmt"
	"io"
	"os"
)

const PathFlag = "--path"

type Entry struct {
	Name string
	Path string
}

type Paths struct {
	entries []Entry
}

func New(entries ...Entry) *Paths {
	return &Paths{entries: entries}
}

func (p *Paths) Report() []string {
	out := make([]string, 0, len(p.entries))
	for _, e := range p.entries {
		status := "not found"
		if exists(e.Path) {
			status = "found"
		}
		out = append(out, fmt.Sprintf("%s: %s (%s)", e.Name, e.Path, status))
	}
	return out
}

func (p *Paths) Fprint(w io.Writer) {
	for _, line := range p.Report() {
		fmt.Fprintln(w, line)
	}
}

func (p *Paths) HandlePath(w io.Writer, args []string) bool {
	if len(args) == 0 || args[0] != PathFlag {
		return false
	}
	p.Fprint(w)
	return true
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
