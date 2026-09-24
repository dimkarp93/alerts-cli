package main

import (
	"fmt"
	"io"
	"runtime/debug"
	"strconv"
	"strings"
)

var (
	version  string
	origin   string
	upstream string
	commit   string
	channel  string
)

func moduleInfo() (path, ver string) {
	if bi, ok := debug.ReadBuildInfo(); ok {
		path = bi.Main.Path
		if bi.Main.Version != "(devel)" {
			ver = bi.Main.Version
		}
	}
	return path, ver
}

func versionString() string {
	if v := strings.TrimSpace(version); v != "" {
		return strings.TrimPrefix(v, "v")
	}
	if _, v := moduleInfo(); v != "" {
		return strings.TrimPrefix(v, "v")
	}
	return "dev"
}

func originString() string {
	if o := strings.TrimSpace(origin); o != "" {
		return normalizeURL(o)
	}
	if p, _ := moduleInfo(); p != "" {
		if i := strings.LastIndex(p, "/v"); i > 0 {
			if _, err := strconv.Atoi(p[i+2:]); err == nil {
				p = p[:i]
			}
		}
		return "https://" + p
	}
	return "unknown"
}

func upstreamString() string {
	if u := strings.TrimSpace(upstream); u != "" {
		return normalizeURL(u)
	}
	return originString()
}

func channelString() string {
	if c := strings.TrimSpace(channel); c != "" {
		return c
	}
	if _, v := moduleInfo(); v != "" {
		return "go-install"
	}
	return ""
}

func normalizeURL(raw string) string {
	u := strings.TrimSpace(raw)
	trim := func(s string) string { return strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git") }
	if i := strings.Index(u, "://"); i >= 0 {
		h := u[i+3:]
		if at := strings.LastIndex(h, "@"); at >= 0 {
			h = h[at+1:]
		}
		return "https://" + trim(h)
	}
	if at := strings.LastIndex(u, "@"); at >= 0 && strings.Contains(u[at+1:], ":") {
		return "https://" + trim(strings.Replace(u[at+1:], ":", "/", 1))
	}
	return "local"
}

func handleBuildFlags(w io.Writer, args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "--version", "-v", "version":
		fmt.Fprintln(w, versionString())
	case "--origin":
		fmt.Fprintln(w, originString())
	case "--buildinfo":
		fmt.Fprintln(w, "origin="+originString())
		fmt.Fprintln(w, "upstream="+upstreamString())
		fmt.Fprintln(w, "version="+versionString())
		if c := strings.TrimSpace(commit); c != "" {
			fmt.Fprintln(w, "commit="+c)
		}
		if c := channelString(); c != "" {
			fmt.Fprintln(w, "channel="+c)
		}
	default:
		return false
	}
	return true
}
