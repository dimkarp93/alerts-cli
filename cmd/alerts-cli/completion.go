package main

import "github.com/dimkarp93/install-libs/shellcomplete"

func commonCompletionFlags(formats, groupBys []string) []shellcomplete.Flag {
	return []shellcomplete.Flag{
		{Name: "-date"},
		{Name: "-severity"},
		{Name: "-format", Values: shellcomplete.Static(formats...)},
		{Name: "-group-by", Values: shellcomplete.Static(groupBys...)},
		{Name: "-tz"},
		{Name: "-col-width"},
		{Name: "-timeout"},
		{Name: "-json-compact", Bool: true},
	}
}

var completionSpec = shellcomplete.Spec{
	Bin: "alerts-cli",
	Flags: []shellcomplete.Flag{
		{Name: "--path", Bool: true},
		{Name: "--version", Bool: true},
		{Name: "-v", Bool: true},
		{Name: "--origin", Bool: true},
		{Name: "--buildinfo", Bool: true},
	},
	Commands: []shellcomplete.Command{
		{Name: "alerts", Flags: append(commonCompletionFlags(alertFormats, alertGroupBys),
			shellcomplete.Flag{Name: "-mode", Values: shellcomplete.Static("all", "unmuted", "muted")},
			shellcomplete.Flag{Name: "-env"},
			shellcomplete.Flag{Name: "-selector"},
			shellcomplete.Flag{Name: "-state", Values: shellcomplete.Static("firing", "pending", "any")},
			shellcomplete.Flag{Name: "-step"},
			shellcomplete.Flag{Name: "-no-silences", Bool: true},
			shellcomplete.Flag{Name: "-labels", Bool: true},
		)},
		{Name: "mutes", Flags: append(commonCompletionFlags(muteFormats, muteGroupBys),
			shellcomplete.Flag{Name: "-filter", Values: shellcomplete.Static("all", "created", "expired")},
			shellcomplete.Flag{Name: "-by"},
			shellcomplete.Flag{Name: "-matchers", Bool: true},
		)},
		{Name: "help"},
	},
}
