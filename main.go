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
	"maps"

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

var barChars = []string{
	"|",
	"#",
	"+",
	"%",
	"-",
	"/",
	".",
	"@",
}

var colorPallets = []string{
    "\x1b[38;5;250m", // other → グレー
    "\x1b[38;5;196m", // 赤
    "\x1b[38;5;46m",  // 緑
    "\x1b[38;5;208m", // オレンジ
    "\x1b[38;5;51m",  // シアン
    "\x1b[38;5;27m",  // 青
    "\x1b[38;5;226m", // 黄
    "\x1b[38;5;201m", // マゼンタ
}

func colorize(s string, idx int) string {
	return colorPallets[idx%len(colorPallets)] + s + "\x1b[0m"
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

type histogram struct {
	base   time.Time
	binVol time.Duration
	binCnt []int
	series map[string][]int
}

func newHistogram(size time.Duration) *histogram {
	return &histogram{
		binCnt: make([]int, 0),
		series: make(map[string][]int, 0),
		binVol: size,
	}
}

func increment(s []int, idx int) []int {
	switch {
	case idx < 0:
		grow := -idx
 		s = append(make([]int, grow), s...)
		s[0] = 1
	case idx >= len(s):
		grow := idx - len(s) + 1
		s = append(s, make([]int, grow)...)
		s[idx] = 1
	default:
		s[idx]++
	}
	return s
}

func (h *histogram) add(t time.Time, label string) {
	if h.base.IsZero() {
		h.base = t.Truncate(h.binVol)
	}

	idx := int(t.Sub(h.base) / h.binVol)
	h.binCnt = increment(h.binCnt, idx)
	h.series[label] = increment(h.series[label], idx)
	if idx < 0 {
		h.base = h.base.Add(-time.Duration(-idx) * h.binVol)
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

	h := newHistogram(opts.interval)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, " ", 2)
		t, err := stringToTime(strings.TrimSpace(parts[0]), opts.format)
		if err != nil {
			continue
		}

		h.add(t, parts[1])
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	var total int
	for _, c := range h.binCnt {
		total += c
	}
	fmt.Printf("Total:   %d items\n", total)
	if total == 0 {
		return nil
	}

	fmt.Printf("Labels:")
	labels := slices.Sorted(maps.Keys(h.series))
	for i, label := range labels {
		fmt.Printf("  %s", colorize(label, i))
	}
	fmt.Println("")
	fmt.Println("")

	m := slices.Max(h.binCnt)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight)
	for i, c := range h.binCnt {
		t := h.base.Add(h.binVol * time.Duration(i))
		ts := t.In(opts.location.Location).Format(time.RFC3339)

		var bar string
		for j, label := range labels {
			chr := colorize("|", j)//barChars[j%len(barChars)]
			if bin, ok := h.series[label]; ok && i < len(bin) {
				bar += strings.Repeat(chr, opts.barlen*bin[i]/m)
				fmt.Printf("%d - %s: %d\n", c, label, bin[i])
			}
		}
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
