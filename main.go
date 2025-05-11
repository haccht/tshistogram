package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	flags "github.com/jessevdk/go-flags"
	"golang.org/x/term"
)

const helpText = `Format Examples:
    ANSIC       "Mon Jan _2 15:04:05 2006"
    UnixDate    "Mon Jan _2 15:04:05 MST 2006"
    RubyDate    "Mon Jan 02 15:04:05 -0700 2006"
    RFC822      "02 Jan 06 15:04 MST"
    RFC822Z     "02 Jan 06 15:04 -0700"
    RFC850      "Monday, 02-Jan-06 15:04:05 MST"
    RFC1123     "Mon, 02 Jan 2006 15:04:05 MST"
    RFC1123Z    "Mon, 02 Jan 2006 15:04:05 -0700"
    RFC3339     "2006-01-02T15:04:05Z07:00"
    RFC3339Nano "2006-01-02T15:04:05.999999999Z07:00"
    Kitchen     "3:04PM"
    Stamp       "Jan _2 15:04:05"
    StampMilli  "Jan _2 15:04:05.000"
    StampMicro  "Jan _2 15:04:05.000000"
    StampNano   "Jan _2 15:04:05.000000000"
    DateTime    "2006-01-02 15:04:05"
    DateOnly    "2006-01-02"
    TimeOnly    "15:04:05"
    Unix        "1136239445"
    Unix-Milli  "1136239445000"
    Unix-Micro  "1136239445000000"

    Arbitrary formats are also supported. See https://pkg.go.dev/time as a reference.`

var layouts = map[string]string{
	"ansic":       time.ANSIC,
	"unixdate":    time.UnixDate,
	"rubydate":    time.RubyDate,
	"rfc822":      time.RFC822,
	"rfc822z":     time.RFC822Z,
	"rfc850":      time.RFC850,
	"rfc1123":     time.RFC1123,
	"rfc1123z":    time.RFC1123Z,
	"rfc3339":     time.RFC3339,
	"rfc3339nano": time.RFC3339Nano,
	"kitchen":     time.Kitchen,
	"stamp":       time.Stamp,
	"stampmilli":  time.StampMilli,
	"stampmicro":  time.StampMicro,
	"stampnano":   time.StampNano,
	"datetime":    time.DateTime,
	"dateonly":    time.DateOnly,
	"timeonly":    time.TimeOnly,
}

var epochLayouts = map[string]int64{
	"unix":       1e6,
	"unix-milli": 1e3,
	"unix-micro": 1,
}

type options struct {
	Format       string `short:"f" long:"format" description:"Time format for parsing the input text (default: guess)"`
	TimeInterval string `short:"i" long:"interval" description:"Time duration for aggregation" default:"5m"`
	TimeFrom     string `long:"time-from" description:"Time to display the chart from"`
	TimeTo       string `long:"time-to" description:"Time to display the chart to"`
	TimeZone     string `long:"time-zone" description:"TimeZone to display the time" default:"UTC"`
	BarLength    int    `long:"barlength" description:"Bar length of the chart" default:"60"`
	BinBound     int    `long:"bound" description:"Upper bound of the chart"`
	Help         bool   `short:"h" long:"help" description:"Show this help message"`
}

type guessRule struct {
	re      *regexp.Regexp
	layouts []string
}

var guessRules = []guessRule{
	{regexp.MustCompile(`^\d{10,19}(?:\.\d+)?$`), []string{"unix", "unix-milli", "unix-micro"}},
	{regexp.MustCompile(`^\d{4}`), []string{"rfc3339", "rfc3339nano", "datetime", "dateonly"}},
	{regexp.MustCompile(`[A-Za-z]{3,4}|[+-]\d{4}`), []string{"unixdate", "rubydate", "rfc822", "rfc822z", "rfc850", "rfc1123", "rfc1123z", "rfc3339", "rfc3339nano"}},
	{regexp.MustCompile(`^[A-Za-z]{3},?`), []string{"ansic", "unixdate", "rubydate", "rfc822", "rfc822z", "rfc850", "rfc1123", "rfc1123z", "stamp", "stampmilli", "stampmicro", "stampnano"}},
	{regexp.MustCompile(`\d{2}:\d{2}:\d{2}`), []string{"datetime", "timeonly", "ansic", "unixdate", "rubydate", "rfc850", "rfc1123", "rfc1123z"}},
	{regexp.MustCompile(`\d{1,2}:\d{2}(AM|PM)`), []string{"kitchen"}},
}

func stringToTime(s, format string) (time.Time, error) {
	if format == "" {
		return guessTime(s)
	}

	if layout, ok := layouts[format]; ok {
		return time.Parse(layout, s)
	}

	if scale, ok := epochLayouts[strings.ToLower(format)]; ok {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse epoch time: %s", s)
		}
		return time.UnixMicro(int64(v * float64(scale))), nil
	}

	return time.Time{}, fmt.Errorf("failed to parse time: %s", s)
}

func guessTime(s string) (time.Time, error) {
	for _, rule := range guessRules {
		if rule.re.MatchString(s) {
			for _, l := range rule.layouts {
				if t, err := stringToTime(s, l); err == nil {
					return t, nil
				}
			}
		}
	}
	return time.Time{}, fmt.Errorf("Unknown format: %s", s)
}

func parseOpts(params []string) (*options, []string, error) {
	var opts options
	parser := flags.NewParser(&opts, flags.Default&^flags.HelpFlag)
	parser.Usage = "[Options]"
	args, err := parser.ParseArgs(params)
	if err != nil {
		return nil, nil, err
	}

	if opts.Help {
		var message bytes.Buffer
		parser.WriteHelp(&message)
		fmt.Fprint(&message, "\n")
		fmt.Fprint(&message, helpText)
		fmt.Fprintln(os.Stdout, message.String())
		os.Exit(0)
	}

	return &opts, args, nil
}

func run() error {
	opts, args, err := parseOpts(os.Args[1:])
	if err != nil {
		return err
	}

	loc, err := time.LoadLocation(opts.TimeZone)
	if err != nil {
		return err
	}

	gap, err := time.ParseDuration(opts.TimeInterval)
	if err != nil {
		return err
	}

	format := strings.ToLower(opts.Format)

	var timeFrom time.Time
	if opts.TimeFrom != "" {
		t, err := stringToTime(opts.TimeFrom, format)
		if err != nil {
			return err
		}
		timeFrom = t.In(loc).Truncate(gap)
	}

	var timeTo time.Time
	if opts.TimeTo != "" {
		t, err := stringToTime(opts.TimeTo, format)
		if err != nil {
			return err
		}
		timeTo = t.In(loc).Truncate(gap).Add(gap)
	}

	readers := make([]io.Reader, 0)
	for _, arg := range args {
		f, err := os.Open(arg)
		if err != nil {
			return err
		}
		defer f.Close()
		readers = append(readers, f)
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		readers = append(readers, os.Stdin)
	}
	if len(readers) == 0 {
		return fmt.Errorf("No input specified")
	}

	items := make([]time.Time, 0, 1024*1024)
	scanner := bufio.NewScanner(io.MultiReader(readers...))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		t, err := stringToTime(line, format)
		if err != nil {
			continue
		}

		items = append(items, t.In(loc))
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	fmt.Printf("Total: %d items\n", len(items))
	if len(items) == 0 {
		return nil
	}

	bins := make(map[time.Time]int, 1024)
	binBound := 0

	timeMin := items[0]
	timeMax := items[0]
	for _, t := range items {
		tt := t.Truncate(gap)
		bins[tt]++

		if binBound < bins[tt] {
			binBound = bins[tt]
		}
		if t.Before(timeMin) {
			timeMin = t
		}
		if t.After(timeMax) {
			timeMax = t
		}
	}
	fmt.Printf("Range: %s - %s\n\n", timeMin.Format(time.RFC3339), timeMax.Format(time.RFC3339))

	if opts.BinBound != 0 {
		binBound = max(binBound, opts.BinBound)
	}
	if timeFrom.IsZero() {
		timeFrom = timeMin.Truncate(gap)
	}
	if timeTo.IsZero() {
		timeTo = timeMax.Truncate(gap).Add(gap)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight)
	for tt := timeFrom; tt.Before(timeTo); tt = tt.Add(gap) {
		if tt.Year() == 0 {
			tt = tt.AddDate(tt.Year(), 0, 0)
		}

		bar := strings.Repeat("|", opts.BarLength*bins[tt]/binBound)
		fmt.Fprintf(w, "[\t%s\t]\t%6d\t  %s\n", tt.Format(time.RFC3339), bins[tt], bar)
	}
	w.Flush()

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
