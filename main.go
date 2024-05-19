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
	"golang.org/x/crypto/ssh/terminal"
)

type options struct {
	Format   string `short:"f" long:"format" description:"Time layout format to parse" default:"RFC3339"`
	Interval string `short:"i" long:"interval" description:"Interval duration for each bins in the histogram" default:"5m"`
	Timezone string `short:"z" long:"tz" description:"Timezone to display" default:"UTC"`
	TimeFrom string `short:"s" long:"from" description:"Time range from"`
	TimeTo   string `short:"e" long:"to" description:"Time range to"`
    Width    int    `short:"w" long:"width" description:"Bar length" default:"40"`
	Help     bool   `short:"h" long:"help" description:"Show this help message"`
}

func parseTime(format, value string) (time.Time, error) {
	switch strings.ToLower(format) {
	case "unix":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse epoch time: %s", value)
		}
		return time.UnixMicro(int64(f * 1000000)), nil
	case "unix.milli":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse epoch time: %s", value)
		}
		return time.UnixMicro(int64(f * 1000)), nil
	case "unix.micro":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse epoch time: %s", value)
		}
		return time.UnixMicro(int64(f)), nil
	case "ansic":
		return time.Parse(time.ANSIC, value)
	case "unixdate":
		return time.Parse(time.UnixDate, value)
	case "rubydate":
		return time.Parse(time.RubyDate, value)
	case "rfc822":
		return time.Parse(time.RFC822, value)
	case "rfc822z":
		return time.Parse(time.RFC822Z, value)
	case "rfc850":
		return time.Parse(time.RFC850, value)
	case "rfc1123":
		return time.Parse(time.RFC1123, value)
	case "rfc1123z":
		return time.Parse(time.RFC1123Z, value)
	case "rfc3339":
		return time.Parse(time.RFC3339, value)
	case "rfc3339nano":
		return time.Parse(time.RFC3339Nano, value)
	case "kitchen":
		return time.Parse(time.Kitchen, value)
	case "stamp":
		return time.Parse(time.Stamp, value)
	case "stampmilli":
		return time.Parse(time.StampMilli, value)
	case "stampmicro":
		return time.Parse(time.StampMicro, value)
	case "stampnano":
		return time.Parse(time.StampNano, value)
	case "datetime":
		return time.Parse(time.DateTime, value)
	case "dateonly":
		return time.Parse(time.DateOnly, value)
	case "timeonly":
		return time.Parse(time.TimeOnly, value)
	default:
		return time.Parse(format, value)
	}
}

func parseOpts(args []string) (*options, []string, error) {
	var o options
	parser := flags.NewParser(&o, flags.Default&^flags.HelpFlag)
	parser.Usage = "[Options]"

	r, err := parser.ParseArgs(args)
	if err != nil {
        return nil, nil, err
	}

	if o.Help {
		var message bytes.Buffer

		parser.WriteHelp(&message)
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
    return &o, r, nil
}

func run() error {
    opts, args, err := parseOpts(os.Args[1:])
    if err != nil {
        fmt.Println(err)
        os.Exit(1)
    }

	loc, err := time.LoadLocation(opts.Timezone)
	if err != nil {
		return err
	}

	wid, err := time.ParseDuration(opts.Interval)
	if err != nil {
		return err
	}

    var min time.Time
	if opts.TimeFrom != "" {
		if t, err := parseTime(opts.Format, opts.TimeFrom); err == nil {
			min = t
		}
	}

    var max time.Time
	if opts.TimeTo != "" {
		if t, err := parseTime(opts.Format, opts.TimeTo); err == nil {
			max = t
		}
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
	if !terminal.IsTerminal(0) {
		readers = append(readers, os.Stdin)
	}
	if len(readers) == 0 {
        return fmt.Errorf("No input specified")
	}

	year := time.Now().Year()
	items := make([]time.Time, 0, 1024*1024)

    sc := bufio.NewScanner(io.MultiReader(readers...))
	for sc.Scan() {
		text := strings.TrimSpace(sc.Text())
		if item, err := parseTime(opts.Format, text); err == nil {
			item = item.In(loc)
			if item.Year() == 0 {
				item = item.AddDate(year, 0, 0)
			}
			items = append(items, item)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	fmt.Printf("Total items = %d\n", len(items))
	if len(items) == 0 {
		return nil
	}

	if min.Equal(time.Time{}) {
        min = items[0]
	}

	if max.Equal(time.Time{}) {
        max = items[0]
	}

	for _, item := range items {
		if min.After(item) {
			min = item
		}
		if max.Before(item) {
			max = item
		}
	}

	tmin := min.Truncate(wid)
	tmax := max.Truncate(wid)

	var wmax int
	bins := make([]int, (tmax.UnixMicro()-tmin.UnixMicro())/wid.Microseconds()+1)
	for _, item := range items {
		idx := int((item.UnixMicro() - tmin.UnixMicro()) / wid.Microseconds())
		bins[idx]++
		if bins[idx] > wmax {
			wmax = bins[idx]
		}
	}
	fmt.Printf("Time range  = %s - %s\n\n", min.Format(time.RFC3339), max.Format(time.RFC3339))

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight)
	for idx, count := range bins {
		bmin := tmin.Add(wid * time.Duration(idx))
		bar := "  " + strings.Repeat("|", opts.Width*count/wmax)

		fmt.Fprintf(tw, "[\t%s\t]\t%6d\t%s\n", bmin.Format(time.RFC3339), count, bar)
	}
	tw.Flush()

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
