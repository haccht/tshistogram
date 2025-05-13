package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/pflag"
)

const layoutExamples = `  ANSIC       "Mon Jan _2 15:04:05 2006"
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

var knownLayouts = map[string]string{
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

type options struct {
	format   string
	interval time.Duration
	barlen   int
	location locationValue
	inputs   []string
}

func parseFlags() *options {
	var opts options
	opts.location.Location = time.Local

	pflag.StringVarP(&opts.format, "format", "f", "", "Input time format (default: auto)")
	pflag.DurationVarP(&opts.interval, "interval", "i", 5*time.Minute, "Bin width as duration (e.g. 30s, 1m, 1h)")
	pflag.IntVarP(&opts.barlen, "barlength", "b", 60, "Length of the longest bar")
	pflag.VarP(&opts.location, "location", "l", "Timezone location (e.g., UTC, Asia/Tokyo)")
	pflag.CommandLine.SortFlags = false
	pflag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintf(os.Stderr, "  %s [Options] [file...]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(os.Stderr, "Options:")
		fmt.Fprintf(os.Stderr, "%s\n", pflag.CommandLine.FlagUsages())
		fmt.Fprintln(os.Stderr, "Format Examples:")
		fmt.Fprintf(os.Stderr, "%s\n", layoutExamples)
		os.Exit(0)
	}

	pflag.Parse()

	opts.inputs = pflag.Args()
	opts.format = strings.ToLower(opts.format)
	return &opts
}

func stringToTime(s, format string) (time.Time, error) {
	if format == "" {
		return guessTime(s)
	}

	if scale, ok := epochLayouts[strings.ToLower(format)]; ok {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse epoch time: %s", s)
		}
		return time.UnixMicro(int64(v * float64(scale))), nil
	}

	if layout, ok := knownLayouts[format]; ok {
		return time.Parse(layout, s)
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
	return time.Time{}, fmt.Errorf("unknown format: %s", s)
}

type timeValue struct {
	time.Time
}

func (tv *timeValue) String() string {
	return tv.Format(time.RFC3339)
}

func (tv *timeValue) Set(value string) error {
	t, err := guessTime(value)
	if err != nil {
		return fmt.Errorf("invalid timestamp %q: %w", value, err)
	}
	tv.Time = t
	return nil
}

func (tv *timeValue) Type() string {
	return "timestamp"
}

type locationValue struct {
	*time.Location
}

func (lv *locationValue) String() string {
	return lv.Location.String()
}

func (lv *locationValue) Set(value string) error {
	loc, err := time.LoadLocation(value)
	if err != nil {
		return fmt.Errorf("invalid location %q: %w", value, err)
	}
	lv.Location = loc
	return nil
}

func (lv *locationValue) Type() string {
	return "location"
}

type bins struct {
	base   time.Time
	size   time.Duration
	total  int
	counts []int
}

func newBins(size time.Duration) *bins {
	return &bins{
		size:   size,
		counts: []int{},
	}
}

func (b *bins) add(t time.Time) {
	if b.base.IsZero() {
		b.base = t.Truncate(b.size)
	}
	idx := int(t.Sub(b.base) / b.size)
	b.total++

	switch {
	case idx < 0:
		grow := -idx
		b.counts = append(make([]int, grow), b.counts...)
		b.base = b.base.Add(-time.Duration(grow) * b.size)
		b.counts[0] = 1
	case idx >= len(b.counts):
		grow := idx - len(b.counts) + 1
		b.counts = append(b.counts, make([]int, grow)...)
		b.counts[idx] = 1
	default:
		b.counts[idx]++
	}
}

func run() error {
	opts := parseFlags()

	var reader io.Reader
	if len(opts.inputs) > 0 {
		readers := make([]io.Reader, 0)
		for _, file := range opts.inputs {
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()
			readers = append(readers, f)
		}
		reader = io.MultiReader(readers...)
	} else {
		reader = os.Stdin
	}

	b := newBins(opts.interval)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		t, err := stringToTime(s, opts.format)
		if err != nil {
			continue
		}

		b.add(t)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	fmt.Printf("Total: %d items\n\n", b.total)
	if b.total == 0 {
		return nil
	}

	m := slices.Max(b.counts)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight)
	for i, c := range b.counts {
		t := b.base.Add(time.Duration(i) * b.size)
		if t.Year() == 0 {
			t = t.AddDate(t.Year(), 0, 0)
		}

		ts := t.In(opts.location.Location).Format(time.RFC3339)
		bar := strings.Repeat("|", opts.barlen*c/m)
		fmt.Fprintf(w, "[\t%s\t]\t%6d\t  %s\n", ts, c, bar)
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
