package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	flags "github.com/jessevdk/go-flags"
	"golang.org/x/term"
)

type options struct {
	TimeFormat   string `short:"f" long:"format" description:"Time layout to parse the input text" default:"RFC3339"`
	TimeInterval string `short:"i" long:"interval" description:"Time interval to split the time range" default:"5m"`
	TimeZone     string `long:"time-zone" description:"Time zone to display the legend" default:"UTC"`
	TimeFrom     string `long:"time-from" description:"Time to display the chart from"`
	TimeTo       string `long:"time-to" description:"Time to display the chart to"`
	BarLength    int    `long:"bar-length" description:"Bar chart length" default:"60"`
	BinSize      int    `long:"ubound" description:"Upper bound of the count"`
	Help         bool   `short:"h" long:"help" description:"Show this help message"`
}

func parseTime(layout, text string, loc *time.Location) (time.Time, error) {
	switch strings.ToLower(layout) {
	case "unix":
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse epoch time: %s", text)
		}
		return time.UnixMicro(int64(f * 1000000)).In(loc), nil
	case "unix.milli":
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse epoch time: %s", text)
		}
		return time.UnixMicro(int64(f * 1000)).In(loc), nil
	case "unix.micro":
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse epoch time: %s", text)
		}
		return time.UnixMicro(int64(f)).In(loc), nil
	case "ansic":
		return time.Parse(time.ANSIC, text)
	case "unixdate":
		return time.Parse(time.UnixDate, text)
	case "rubydate":
		return time.Parse(time.RubyDate, text)
	case "rfc822":
		return time.Parse(time.RFC822, text)
	case "rfc822z":
		return time.Parse(time.RFC822Z, text)
	case "rfc850":
		return time.Parse(time.RFC850, text)
	case "rfc1123":
		return time.Parse(time.RFC1123, text)
	case "rfc1123z":
		return time.Parse(time.RFC1123Z, text)
	case "rfc3339":
		return time.Parse(time.RFC3339, text)
	case "rfc3339nano":
		return time.Parse(time.RFC3339Nano, text)
	case "kitchen":
		return time.Parse(time.Kitchen, text)
	case "stamp":
		return time.Parse(time.Stamp, text)
	case "stampmilli":
		return time.Parse(time.StampMilli, text)
	case "stampmicro":
		return time.Parse(time.StampMicro, text)
	case "stampnano":
		return time.Parse(time.StampNano, text)
	case "datetime":
		return time.Parse(time.DateTime, text)
	case "dateonly":
		return time.Parse(time.DateOnly, text)
	case "timeonly":
		return time.Parse(time.TimeOnly, text)
	default:
		return time.Parse(layout, text)
	}
}

func parseOpts(params []string) (*options, []string, error) {
	var opts options

	fp := flags.NewParser(&opts, flags.Default&^flags.HelpFlag)
	fp.Usage = "[Options]"

	args, err := fp.ParseArgs(params)
	if err != nil {
		return nil, nil, err
	}

	if opts.Help {
		var message bytes.Buffer

		fp.WriteHelp(&message)
		fmt.Fprint(&message, `
Format Examples:
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

    Arbitrary formats are also supported. See https://pkg.go.dev/time as a reference.`)

		fmt.Println(message.String())
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

	var timeFrom time.Time
	if opts.TimeFrom != "" {
		t, err := parseTime(opts.TimeFormat, opts.TimeFrom, loc)
		if err != nil {
			return err
		}
		timeFrom = t
	}

	var timeTo time.Time
	if opts.TimeTo != "" {
		t, err := parseTime(opts.TimeFormat, opts.TimeTo, loc)
		if err != nil {
			return err
		}
		timeTo = t
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
		t, err := parseTime(opts.TimeFormat, line, loc)
		if err != nil {
			continue
		}

		items = append(items, t)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	fmt.Printf("Total: %d items\n", len(items))
	if len(items) == 0 {
		return nil
	}

	bins := make(map[time.Time]int, 1024)
	binSize := 0

	timeMin := items[0]
	timeMax := items[0]
	for _, t := range items {
		tt := t.Truncate(gap)
		bins[tt]++

		if binSize < bins[tt] {
			binSize = bins[tt]
		}

		if t.Before(timeMin) {
			timeMin = t
		}
		if t.After(timeMax) {
			timeMax = t
		}
	}

	if opts.BinSize != 0 {
		binSize = max(binSize, opts.BinSize)
	}
	if timeFrom.IsZero() {
		timeFrom = timeMin.Truncate(gap)
	}
	if timeTo.IsZero() {
		timeTo = timeMax.Truncate(gap).Add(gap)
	}
	fmt.Printf("Range: %s - %s\n\n", timeMin.Format(time.RFC3339), timeMax.Format(time.RFC3339))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight)
	for tt := timeFrom; tt.Before(timeTo); tt = tt.Add(gap) {
		if tt.Year() == 0 {
			tt = tt.AddDate(tt.Year(), 0, 0)
		}

		bar := strings.Repeat("|", bins[tt]*opts.BarLength/binSize)
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
