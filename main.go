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
	TimeLayout   string `short:"f" long:"format" description:"Time format for parsing the input text" default:"RFC3339"`
	TimeInterval string `short:"i" long:"interval" description:"Time duration for aggregation" default:"5m"`
	TimeFrom     string `long:"time-from" description:"Time to display the chart from"`
	TimeTo       string `long:"time-to" description:"Time to display the chart to"`
	TimeZone     string `long:"time-zone" description:"TimeZone to display the time" default:"UTC"`
	BarLength    int    `long:"barlength" description:"Bar length of the chart" default:"60"`
	BinBound     int    `long:"bound" description:"Upper bound of the chart"`
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
		return time.ParseInLocation(time.ANSIC, text, loc)
	case "unixdate":
		return time.ParseInLocation(time.UnixDate, text, loc)
	case "rubydate":
		return time.ParseInLocation(time.RubyDate, text, loc)
	case "rfc822":
		return time.ParseInLocation(time.RFC822, text, loc)
	case "rfc822z":
		return time.ParseInLocation(time.RFC822Z, text, loc)
	case "rfc850":
		return time.ParseInLocation(time.RFC850, text, loc)
	case "rfc1123":
		return time.ParseInLocation(time.RFC1123, text, loc)
	case "rfc1123z":
		return time.ParseInLocation(time.RFC1123Z, text, loc)
	case "rfc3339":
		return time.ParseInLocation(time.RFC3339, text, loc)
	case "rfc3339nano":
		return time.ParseInLocation(time.RFC3339Nano, text, loc)
	case "kitchen":
		return time.ParseInLocation(time.Kitchen, text, loc)
	case "stamp":
		return time.ParseInLocation(time.Stamp, text, loc)
	case "stampmilli":
		return time.ParseInLocation(time.StampMilli, text, loc)
	case "stampmicro":
		return time.ParseInLocation(time.StampMicro, text, loc)
	case "stampnano":
		return time.ParseInLocation(time.StampNano, text, loc)
	case "datetime":
		return time.ParseInLocation(time.DateTime, text, loc)
	case "dateonly":
		return time.ParseInLocation(time.DateOnly, text, loc)
	case "timeonly":
		return time.ParseInLocation(time.TimeOnly, text, loc)
	default:
		return time.ParseInLocation(layout, text, loc)
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
		t, err := parseTime(opts.TimeLayout, opts.TimeFrom, loc)
		if err != nil {
			return err
		}
		timeFrom = t.Truncate(gap)
	}

	var timeTo time.Time
	if opts.TimeTo != "" {
		t, err := parseTime(opts.TimeLayout, opts.TimeTo, loc)
		if err != nil {
			return err
		}
		timeTo = t.Truncate(gap).Add(gap)
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
		t, err := parseTime(opts.TimeLayout, line, loc)
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
