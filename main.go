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
	style    string
	inputs   []string
}

func parseFlags() *options {
	var opts options
	opts.location.Location = time.Local

	pflag.StringVarP(&opts.format, "format", "f", "", "Input time format (default: auto)")
	pflag.DurationVarP(&opts.interval, "interval", "i", 5*time.Minute, "Bin width as duration (e.g. 30s, 1m, 1h)")
	pflag.IntVarP(&opts.barlen, "barlength", "b", 60, "Length of the longest bar")
	pflag.VarP(&opts.location, "location", "l", "Timezone location (e.g., UTC, Asia/Tokyo)")
	pflag.StringVar(&opts.style, "style", "char", "Output style: 'char' for characters, 'color' for ANSI colors")
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
	counts []map[string]int
	series map[string]bool
}

func newBins(size time.Duration) *bins {
	return &bins{
		size:   size,
		counts: []map[string]int{},
		series: make(map[string]bool),
	}
}

func (b *bins) add(t time.Time, seriesName string) {
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

var barChars = []rune{'|', '#', '=', '*', '+', '@', '$', '%', '&'}

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
		fields := strings.Fields(strings.TrimSpace(scanner.Text()))
		if len(fields) == 0 {
			continue
		}
		timeStr := fields[0]
		seriesName := ""
		if len(fields) > 1 {
			seriesName = fields[1]
		}

		t, err := stringToTime(timeStr, opts.format)
		if err != nil {
			continue
		}

		b.add(t, seriesName)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	fmt.Printf("Total: %d items\n\n", b.total)
	if b.total == 0 {
		return nil
	}

	seriesNames := make([]string, 0, len(b.series))
	for name := range b.series {
		seriesNames = append(seriesNames, name)
	}
	slices.Sort(seriesNames)

	fmt.Println("Legend:")
	charStyles := make(map[string]rune)
	colorStyles := make(map[string]string)
	for i, name := range seriesNames {
		displayName := name
		if name == "" {
			displayName = "(default)"
		}
		if opts.style == "char" {
			char := barChars[i%len(barChars)]
			charStyles[name] = char
			fmt.Printf("  %c: %s\n", char, displayName)
		} else { // color style
			color := barColors[i%len(barColors)]
			colorStyles[name] = color
			fmt.Printf("  %s|%s: %s\n", color, colorReset, displayName)
		}
	}
	fmt.Println()

	maxTotalInBin := 0
	for _, seriesCounts := range b.counts {
		currentTotal := 0
		if seriesCounts != nil {
			for _, count := range seriesCounts {
				currentTotal += count
			}
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
		if t.Year() == 0 {
			t = t.AddDate(t.Year(), 0, 0)
		}
		ts := t.In(opts.location.Location).Format(time.RFC3339)

		totalInBin := 0
		if seriesCounts != nil {
			for _, count := range seriesCounts {
				totalInBin += count
			}
		}

		if totalInBin == 0 {
			fmt.Fprintf(w, "[\t%s\t]\t%6d\t  %s\n", ts, 0, "")
			continue
		}

		currentBarTotalLen := (opts.barlen * totalInBin) / maxTotalInBin

		barLens := make(map[string]int)
		remainders := make(map[string]int)
		totalLenAssigned := 0

		for _, seriesName := range seriesNames {
			if count, ok := seriesCounts[seriesName]; ok {
				val := (count * currentBarTotalLen * 100) / totalInBin
				barLens[seriesName] = val / 100
				remainders[seriesName] = val % 100
				totalLenAssigned += barLens[seriesName]
			}
		}

		lenToDistribute := currentBarTotalLen - totalLenAssigned

		sortedByRemainder := make([]string, 0, len(remainders))
		for name := range remainders {
			sortedByRemainder = append(sortedByRemainder, name)
		}
		slices.SortStableFunc(sortedByRemainder, func(a, b string) int {
			return remainders[b] - remainders[a]
		})

		for i := 0; i < lenToDistribute; i++ {
			seriesToIncrement := sortedByRemainder[i%len(sortedByRemainder)]
			barLens[seriesToIncrement]++
		}

		var barBuilder strings.Builder
		for _, seriesName := range seriesNames {
			if barPartLen, ok := barLens[seriesName]; ok && barPartLen > 0 {
				if opts.style == "char" {
					barBuilder.WriteString(strings.Repeat(string(charStyles[seriesName]), barPartLen))
				} else {
					barBuilder.WriteString(colorStyles[seriesName])
					barBuilder.WriteString(strings.Repeat("|", barPartLen))
					barBuilder.WriteString(colorReset)
				}
			}
		}

		fmt.Fprintf(w, "[\t%s\t]\t%6d\t  %s\n", ts, totalInBin, barBuilder.String())
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
