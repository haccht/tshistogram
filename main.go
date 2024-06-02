package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	flags "github.com/jessevdk/go-flags"
	//"github.com/muesli/termenv"
	"golang.org/x/term"
)

type Options struct {
	Format    string `short:"f" long:"format" description:"Time layout format to parse" default:"RFC3339"`
	Interval  string `short:"i" long:"interval" description:"Interval duration for each bins in the histogram" default:"5m"`
	TimeZone  string `short:"z" long:"tz" description:"Timezone to display" default:"UTC"`
	BarLength int    `long:"bar-length" description:"Maximum bar length" default:"60"`
	BinSize   int    `long:"bin-size" description:"Maximum bin size"`
	TimeFrom  string `long:"from" description:"Time range from"`
	TimeTo    string `long:"to" description:"Time range to"`
	Help      bool   `short:"h" long:"help" description:"Show this help message"`
}

type item struct {
	ts   time.Time
	text string
}

type bin struct {
	wc map[string]int
}

func parsePrefixFloat(value string) (float64, string, error) {
	v, suffix, _ := strings.Cut(value, " ")
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, "", fmt.Errorf("failed to parse epoch time: %s", v)
	}
	return f, strings.TrimSpace(suffix), nil
}

func parsePrefixedTime(layout, value string, loc *time.Location) (time.Time, string, error) {
	t, err := time.ParseInLocation(layout, value, loc)
	if e, ok := err.(*time.ParseError); ok && strings.HasPrefix(e.Message, ": extra text: ") {
		prefix, suffix := value[:len(value)-len(e.ValueElem)], value[len(value)-len(e.ValueElem):]
		t, _ = time.ParseInLocation(layout, prefix, loc)
		return t, strings.TrimSpace(suffix), nil
	}
	return t, "", err
}

func parseTime(layout, value string, loc *time.Location) (time.Time, string, error) {
	switch strings.ToLower(layout) {
	case "unix":
		f, suffix, err := parsePrefixFloat(value)
		if err != nil {
			return time.Time{}, "", err
		}
		return time.UnixMicro(int64(f * 1000000)).In(loc), suffix, nil
	case "unix.milli":
		f, suffix, err := parsePrefixFloat(value)
		if err != nil {
			return time.Time{}, "", err
		}
		return time.UnixMicro(int64(f * 1000)).In(loc), suffix, nil
	case "unix.micro":
		f, suffix, err := parsePrefixFloat(value)
		if err != nil {
			return time.Time{}, "", err
		}
		return time.UnixMicro(int64(f)).In(loc), suffix, nil
	case "ansic":
		return parsePrefixedTime(time.ANSIC, value, loc)
	case "unixdate":
		return parsePrefixedTime(time.UnixDate, value, loc)
	case "rubydate":
		return parsePrefixedTime(time.RubyDate, value, loc)
	case "rfc822":
		return parsePrefixedTime(time.RFC822, value, loc)
	case "rfc822z":
		return parsePrefixedTime(time.RFC822Z, value, loc)
	case "rfc850":
		return parsePrefixedTime(time.RFC850, value, loc)
	case "rfc1123":
		return parsePrefixedTime(time.RFC1123, value, loc)
	case "rfc1123z":
		return parsePrefixedTime(time.RFC1123Z, value, loc)
	case "rfc3339":
		return parsePrefixedTime(time.RFC3339, value, loc)
	case "rfc3339nano":
		return parsePrefixedTime(time.RFC3339Nano, value, loc)
	case "kitchen":
		return parsePrefixedTime(time.Kitchen, value, loc)
	case "stamp":
		return parsePrefixedTime(time.Stamp, value, loc)
	case "stampmilli":
		return parsePrefixedTime(time.StampMilli, value, loc)
	case "stampmicro":
		return parsePrefixedTime(time.StampMicro, value, loc)
	case "stampnano":
		return parsePrefixedTime(time.StampNano, value, loc)
	case "datetime":
		return parsePrefixedTime(time.DateTime, value, loc)
	case "dateonly":
		return parsePrefixedTime(time.DateOnly, value, loc)
	case "timeonly":
		return parsePrefixedTime(time.TimeOnly, value, loc)
	default:
		return parsePrefixedTime(layout, value, loc)
	}
}

func parseOptions(params []string) (*Options, []string, error) {
	var opts Options

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
	opts, args, err := parseOptions(os.Args[1:])
	if err != nil {
		return err
	}

	loc, err := time.LoadLocation(opts.TimeZone)
	if err != nil {
		return err
	}

	gap, err := time.ParseDuration(opts.Interval)
	if err != nil {
		return err
	}

	var timeFrom time.Time
	if opts.TimeFrom != "" {
		t, _, err := parseTime(opts.Format, opts.TimeFrom, loc)
		if err != nil {
			return err
		}
		timeFrom = t
	}

	var timeTo time.Time
	if opts.TimeTo != "" {
		t, _, err := parseTime(opts.Format, opts.TimeTo, loc)
		if err != nil {
			return err
		}
		timeTo = t
	}

	//terminal := termenv.ColorProfile()
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

	items := make([]*item, 0, 1024*1024)
	scanner := bufio.NewScanner(io.MultiReader(readers...))
	for scanner.Scan() {
		t, suffix, err := parseTime(opts.Format, scanner.Text(), loc)
		if err != nil {
			continue
		}
		items = append(items, &item{t, suffix})
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	fmt.Printf("Total: %d items\n", len(items))
	if len(items) == 0 {
		return nil
	}

	timeMin := items[0].ts
	timeMax := items[0].ts

	tags := make(map[string]int, 8)
	bins := make(map[time.Time]map[string]int, 256)
	for _, item := range items {
		if item.ts.Before(timeMin) {
			timeMin = item.ts
		}
		if item.ts.After(timeMax) {
			timeMax = item.ts
		}

		tt := item.ts.Truncate(gap)
		bin, ok := bins[tt]
		if !ok {
			bin = make(map[string]int, 8)
			bins[tt] = bin
		}

		bin[item.text]++
		tags[item.text]++
	}

	if timeFrom.IsZero() {
		timeFrom = timeMin.Truncate(gap)
	}
	if timeTo.IsZero() {
		timeTo = timeMax.Truncate(gap)
	}
	fmt.Printf("Range: %s - %s\n", timeMin.Format(time.RFC3339), timeMax.Format(time.RFC3339))

	tagList := make([]string, 0, len(tags))
	for k := range tags {
		tagList = append(tagList, k)
	}
	sort.Slice(tagList, func(i, j int) bool { return tags[tagList[i]] > tags[tagList[j]] })

	tagListSize := min(len(tagList), 8)
	if tagListSize != 1 || tagList[0] != "" {
		fmt.Printf("Categories:\n")
		for i, tag := range tagList[:tagListSize] {
			if len(tagList) <= 7 || i != 7 {
				fmt.Printf("    |  %s\n", tag)
			} else {
				fmt.Printf("    |  %s\n", "...others")
			}
		}
	}

	binSize := opts.BinSize
	if binSize == 0 {
		for _, bin := range bins {
			for _, c := range bin {
				if binSize < c {
					binSize = c
				}
			}
		}
	}

	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight)
	for tt := timeFrom.Truncate(gap); tt.Before(timeTo.Truncate(gap).Add(gap)); tt = tt.Add(gap) {
		bin := bins[tt]
		for i, tag := range tagList[:tagListSize] {
			rbar := strings.Repeat("|", opts.BarLength*bin[tag]/binSize)
			count := bin[tag]

			switch {
			case i == 0:
				fmt.Fprintf(w, "[\t%s\t]\t%6d\t  %s\n", tt.Format(time.RFC3339), count, rbar)
			case i == 7 && len(tagList) > 7:
				count = 0
				for _, t := range tagList[7:] {
					count += bin[t]
				}
				fmt.Fprintf(w, "\t\t\t%6d\t  %s\n", count, rbar)
			default:
				fmt.Fprintf(w, "\t\t\t%6d\t  %s\n", count, rbar)
			}
		}
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
