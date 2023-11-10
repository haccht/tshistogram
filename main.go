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
	Interval string `short:"i" long:"interval" description:"Interval duration for each bins in the histogram" default:"5m"`
	Format   string `short:"f" long:"format" description:"Time layout format to parse" default:"RFC3339"`
	Timezone string `short:"z" long:"tz" description:"Override timezone"`
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

func run() error {
	var opts options

	parser := flags.NewParser(&opts, flags.Default&^flags.HelpFlag)
	parser.Usage = "[Options]"

	args, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}

	if opts.Help {
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

	readers := make([]io.Reader, 0, len(args)+1)
	for _, arg := range args {
		f, err := os.Open(arg)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %s", arg, err)
		}
		defer f.Close()
		readers = append(readers, f)
	}
	if !terminal.IsTerminal(0) {
		readers = append(readers, os.Stdin)
	}
	if len(readers) == 0 {
		os.Exit(0)
	}

	now := time.Now()
	plots := make([]time.Time, 0, 1024*1024)

	loc := time.UTC
	if opts.Timezone != "" {
		l, err := time.LoadLocation(opts.Timezone)
		if err != nil {
			return err
		}
		loc = l
	}

	scanner := bufio.NewScanner(io.MultiReader(readers...))
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if plot, err := parseTime(opts.Format, text); err == nil {
			plot = plot.In(loc)
			if plot.Year() == 0 {
				plot = plot.AddDate(now.Year(), 0, 0)
			}
			plots = append(plots, plot)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if len(plots) == 0 {
		fmt.Printf("Total plots = 0\n")
		return nil
	}

	w, err := time.ParseDuration(opts.Interval)
	if err != nil {
		return fmt.Errorf("Unable to parse interval: %s", err)
	}

	min, max := plots[0], plots[0]
	for _, plot := range plots {
		if min.After(plot) {
			min = plot
		}
		if max.Before(plot) {
			max = plot
		}
	}
	tmin := min.Truncate(w)
	tmax := max.Truncate(w)

	var mcount int
	bins := make([]int, (tmax.UnixMicro()-tmin.UnixMicro())/w.Microseconds()+1)
	for _, plot := range plots {
		idx := int((plot.UnixMicro() - tmin.UnixMicro()) / w.Microseconds())

		bins[idx]++
		if bins[idx] > mcount {
			mcount = bins[idx]
		}
	}

	fmt.Printf("Total plots = %d\n", len(plots))
	fmt.Printf("Time range  = %s - %s\n\n", min.Format(time.RFC3339), max.Format(time.RFC3339))

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight)
	for idx, count := range bins {
		bmin := tmin.Add(w * time.Duration(idx))
		bar := "  " + strings.Repeat("|", 40*count/mcount)

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
