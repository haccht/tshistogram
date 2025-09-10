package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"maps"
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

var barChars = []rune{'|', 'x', 'o', '*', '+', '@', '$', '%', '-', '&', '=', '/'}

var barColors = []string{
	"\x1b[31m", // Red
	"\x1b[32m", // Green
	"\x1b[33m", // Yellow
	"\x1b[34m", // Blue
	"\x1b[35m", // Magenta
	"\x1b[36m", // Cyan
	"\x1b[91m", // Bright Red
	"\x1b[92m", // Bright Green
	"\x1b[93m", // Bright Yellow
	"\x1b[94m", // Bright Blue
	"\x1b[95m", // Bright Magenta
	"\x1b[96m", // Bright Cyan
}
const colorReset = "\x1b[0m"

type options struct {
	format   string
	interval time.Duration
	barlen   int
	location locationValue
	color    string
	reader   io.Reader
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

func parseFlags() (*options, error) {
	var opts options
	opts.location.Location = time.Local

	pflag.StringVarP(&opts.format, "format", "f", "", "Input time format (default: auto)")
	pflag.DurationVarP(&opts.interval, "interval", "i", 5*time.Minute, "Bin width as duration (e.g. 30s, 1m, 1h)")
	pflag.IntVarP(&opts.barlen, "barlength", "b", 60, "Length of the longest bar")
	pflag.VarP(&opts.location, "location", "l", "Timezone location (e.g., UTC, Asia/Tokyo)")
	pflag.StringVar(&opts.color, "color", "auto", "Markup the bar: 'never', 'always', 'auto'")

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
	reader, err := genReader(pflag.Args())
	if err != nil {
		return nil, err
	}
	opts.reader= reader
	opts.format = strings.ToLower(opts.format)

	return &opts, nil
}

func genReader(inputs []string) (io.Reader, error) {
	if len(inputs) == 0 {
		return os.Stdin, nil
	}

	readers := make([]io.Reader, 0)
	for _, file := range inputs {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		readers = append(readers, f)
	}
	return io.MultiReader(readers...), nil
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

type bins struct {
	base    time.Time
	size    time.Duration
	total   int
	counts  []map[string]int
	series  map[string]bool
	minTime time.Time
	maxTime time.Time
}

func newBins(size time.Duration) *bins {
	return &bins{
		size:   size,
		counts: []map[string]int{},
		series: make(map[string]bool),
	}
}

func (b *bins) add(t time.Time, seriesName string) {
	if b.minTime.IsZero() || t.Before(b.minTime) {
		b.minTime = t
	}
	if b.maxTime.IsZero() || t.After(b.maxTime) {
		b.maxTime = t
	}

	if b.base.IsZero() {
		b.base = t.Truncate(b.size)
	}

	idx := int(t.Sub(b.base) / b.size)
	b.total++
	b.series[seriesName] = true

	switch {
	case idx < 0:
		grow := -idx
		newCounts := make([]map[string]int, grow)
		b.counts = append(newCounts, b.counts...)
		b.base = b.base.Add(-time.Duration(grow) * b.size)
		b.counts[0] = map[string]int{seriesName: 1}
	case idx >= len(b.counts):
		grow := idx - len(b.counts) + 1
		b.counts = append(b.counts, make([]map[string]int, grow)...)
		b.counts[idx] = map[string]int{seriesName: 1}
	default:
		if b.counts[idx] == nil {
			b.counts[idx] = make(map[string]int)
		}
		b.counts[idx][seriesName]++
	}
}

func run() error {
	opts, err := parseFlags()
	if err != nil {
		return err
	}

	b := newBins(opts.interval)
	scanner := bufio.NewScanner(opts.reader)
	for scanner.Scan() {
		fields := strings.Fields(strings.TrimSpace(scanner.Text()))
		if len(fields) == 0 {
			continue
		}

		t, err := stringToTime(fields[0], opts.format)
		if err != nil {
			continue
		}
		t = t.In(opts.location.Location)
		if t.Year() == 0 {
			t = t.AddDate(t.Year(), 0, 0)
		}

		seriesName := ""
		if len(fields) > 1 {
			seriesName = fields[1]
		}

		b.add(t, seriesName)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if b.total == 0 {
		fmt.Println("Total count = 0")
		return nil
	}

	var style string
	switch opts.color {
	case "always":
		style = "color"
	case "never":
		style = "char"
	default: // "auto"
		if len(b.series) > 1 {
			style = "color"
		} else {
			style = "char"
		}
	}

	fmt.Printf("Total count: %d\n", b.total)
	fmt.Printf("Time range:  %s - %s\n", b.minTime.Format(time.RFC3339), b.maxTime.Format(time.RFC3339))

	seriesNames := slices.Collect(maps.Keys(b.series))
	slices.Sort(seriesNames)

	charStyles := make(map[string]rune)
	colorStyles := make(map[string]string)
	for i, name := range seriesNames {
		if style == "char" {
			charStyles[name] = barChars[i%len(barChars)]
		} else {
			colorStyles[name] = barColors[i%len(barColors)]
		}
	}

	if len(seriesNames) != 1 || seriesNames[0] != "" {
		fmt.Println("Legend:")
		for _, name := range seriesNames {
			if style == "color" {
				fmt.Printf("    %s|%s = %s\n", colorStyles[name], colorReset, name)
			} else {
				fmt.Printf("    %c = %s\n", charStyles[name], name)
			}
		}
	}
	fmt.Println()

	maxTotalInBin := 0
	for _, seriesCounts := range b.counts {
		currentTotal := 0
		for _, count := range seriesCounts {
			currentTotal += count
		}
		if currentTotal > maxTotalInBin {
			maxTotalInBin = currentTotal
		}
	}
	if maxTotalInBin == 0 {
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight)
	for i, seriesCounts := range b.counts {
		t := b.base.Add(time.Duration(i) * b.size)

		totalInBin := 0
		for _, count := range seriesCounts {
			totalInBin += count
		}
		if totalInBin == 0 {
			fmt.Fprintf(w, "[\t%s\t]\t%6d\t  %s\n", t.Format(time.RFC3339), 0, "")
			continue
		}

		barLen := opts.barlen * totalInBin / maxTotalInBin
		barLens := make(map[string]int)

		assignedBarLen := 0
		fractionBarLens := make(map[string]int)

		for _, seriesName := range seriesNames {
			if count, ok := seriesCounts[seriesName]; ok {
				val := (count * barLen * 100) / totalInBin
				barLens[seriesName] = val / 100
				fractionBarLens[seriesName] = val % 100
				assignedBarLen += barLens[seriesName]
			}
		}

		fractionSeriesNames := slices.Collect(maps.Keys(fractionBarLens))
		slices.SortStableFunc(fractionSeriesNames, func(a, b string) int {
			return fractionBarLens[b] - fractionBarLens[a]
		})
		for i := range (barLen - assignedBarLen) {
			seriesToIncrement := fractionSeriesNames[i%len(fractionSeriesNames)]
			barLens[seriesToIncrement]++
		}

		var barBuilder strings.Builder
		for _, seriesName := range seriesNames {
			if barPartLen, ok := barLens[seriesName]; ok && barPartLen > 0 {
				if style == "color" {
					barBuilder.WriteString(colorStyles[seriesName])
					barBuilder.WriteString(strings.Repeat("|", barPartLen))
					barBuilder.WriteString(colorReset)
				} else {
					barBuilder.WriteString(strings.Repeat(string(charStyles[seriesName]), barPartLen))
				}
			}
		}

		fmt.Fprintf(w, "[\t%s\t]\t%6d\t  %s\n", t.Format(time.RFC3339), totalInBin, barBuilder.String())
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
