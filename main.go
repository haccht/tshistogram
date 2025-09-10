package main

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"os"
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
	{regexp.MustCompile(`^\d{10,19}(?:\.\d+)?`), []string{"unix", "unix-milli", "unix-micro"}},
	{regexp.MustCompile(`^\d{4}`), []string{"rfc3339", "rfc3339nano", "datetime", "dateonly"}},
	{regexp.MustCompile(`[A-Za-z]{3,4}|[+-]\d{4}`), []string{"unixdate", "rubydate", "rfc822", "rfc822z", "rfc850", "rfc1123", "rfc1123z", "rfc3339", "rfc3339nano"}},
	{regexp.MustCompile(`^[A-Za-z]{3},?`), []string{"ansic", "unixdate", "rubydate", "rfc822", "rfc822z", "rfc850", "rfc1123", "rfc1123z", "stamp", "stampmilli", "stampmicro", "stampnano"}},
	{regexp.MustCompile(`\d{2}:\d{2}:\d{2}`), []string{"datetime", "timeonly", "ansic", "unixdate", "rubydate", "rfc850", "rfc1123", "rfc1123z"}},
	{regexp.MustCompile(`\d{1,2}:\d{2}(AM|PM)`), []string{"kitchen"}},
}

type barStyle struct {
	char  string
	color string
}

var barStyles = []barStyle{
	{"|", "\x1b[31m"}, // Red
	{"x", "\x1b[32m"}, // Green
	{"o", "\x1b[33m"}, // Yellow
	{"*", "\x1b[34m"}, // Blue
	{"+", "\x1b[35m"}, // Magenta
	{"#", "\x1b[36m"}, // Cyan
}

const blockBarChar = "▇"
const barColorReset = "\x1b[0m"
const otherSeriesName = "(Other)"

type barStyleOption int

const (
	barCharStyle barStyleOption = iota
	barColorStyle
)

type options struct {
	format   string
	interval time.Duration
	barlen   int
	location locationValue
	color    string
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
		return err
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
	pflag.IntVarP(&opts.barlen, "barlength", "b", 120, "Length of the longest bar")
	pflag.VarP(&opts.location, "location", "l", "Timezone location (e.g., UTC, Asia/Tokyo)")
	pflag.StringVar(&opts.color, "color", "auto", "Markup bar color [never|always|auto]")

	pflag.CommandLine.SortFlags = false
	pflag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintf(os.Stderr, "  tshistogram [Options] [file...]\n\n")
		fmt.Fprintln(os.Stderr, "Options:")
		fmt.Fprintf(os.Stderr, "%s\n", pflag.CommandLine.FlagUsages())
		fmt.Fprintln(os.Stderr, "Format Examples:")
		fmt.Fprintf(os.Stderr, "%s\n", layoutExamples)
		os.Exit(0)
	}

	pflag.Parse()
	return &opts, nil
}

type multiFileReader struct {
	reader io.Reader
	files  []*os.File
}

func (mfr *multiFileReader) Read(p []byte) (n int, err error) {
	return mfr.reader.Read(p)
}

func (mfr *multiFileReader) Close() error {
	var closeErrors []error
	for _, f := range mfr.files {
		if err := f.Close(); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	if len(closeErrors) > 0 {
		return fmt.Errorf("failed to close files: %v", closeErrors)
	}
	return nil
}

func genReader(inputs []string) (io.Reader, error) {
	if len(inputs) == 0 {
		return os.Stdin, nil
	}

	files := make([]*os.File, len(inputs))
	for i, file := range inputs {
		f, err := os.Open(file)
		if err != nil {
			for j := 0; j < i; j++ {
				files[j].Close()
			}
			return nil, err
		}
		files[i] = f
	}

	readers := make([]io.Reader, len(files))
	for i, f := range files {
		readers[i] = f
	}

	return &multiFileReader{
		reader: io.MultiReader(readers...),
		files:  files,
	}, nil
}

func stringToTime(s, format string) (time.Time, error) {
	lformat := strings.ToLower(format)
	if lformat == "" {
		return guessTime(s)
	}

	if scale, ok := epochLayouts[lformat]; ok {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse epoch time: %s", s)
		}
		return time.UnixMicro(int64(v * float64(scale))), nil
	}

	if layout, ok := knownLayouts[lformat]; ok {
		return time.Parse(layout, s)
	}

	return time.Parse(format, s)
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

func parseLeadingTime(s, format string) (time.Time, string) {
	fields := strings.Split(s, " ")

	for i := range len(fields) {
		part1 := strings.Join(fields[:i+1], " ")
		part2 := strings.Join(fields[i+1:], " ")

		t, err := stringToTime(part1, format)
		if err == nil {
			return t, part2
		}
	}

	return time.Time{}, s
}

type bins struct {
	base    time.Time
	size    time.Duration
	total   int
	counts  []map[string]int
	series  map[string]struct{}
	minTime time.Time
	maxTime time.Time
}

func newBins(size time.Duration) *bins {
	return &bins{
		size:   size,
		counts: []map[string]int{},
		series: make(map[string]struct{}),
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
	b.series[seriesName] = struct{}{}

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

	reader, err := genReader(pflag.Args())
	if err != nil {
		return err
	}
	if c, ok := reader.(io.Closer); ok {
		defer c.Close()
	}

	b := newBins(opts.interval)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		t, seriesName := parseLeadingTime(line, opts.format)
		if t.IsZero() {
			continue
		}

		t = t.In(opts.location.Location)
		if t.Year() == 0 {
			t = t.AddDate(time.Now().Year(), 0, 0)
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

	var style barStyleOption
	switch opts.color {
	case "always":
		style = barColorStyle
	case "never":
		style = barCharStyle
	case "auto":
		if len(b.series) > 1 {
			style = barColorStyle
		} else {
			style = barCharStyle
		}
	default:
		return fmt.Errorf("invalid color \"%s\"", opts.color)
	}

	var seriesNames []string
	var seriesLimit = len(barStyles) + 1

	if len(b.series) > seriesLimit {
		seriesTotals := make(map[string]int)
		for _, binCounts := range b.counts {
			for seriesName, count := range binCounts {
				seriesTotals[seriesName] += count
			}
		}

		allSeriesNames := slices.Collect(maps.Keys(b.series))
		slices.SortFunc(allSeriesNames, func(a, b string) int {
			if seriesTotals[b] != seriesTotals[a] {
				return seriesTotals[b] - seriesTotals[a]
			}
			return strings.Compare(a, b)
		})

		topSeries := allSeriesNames[:seriesLimit-2]
		otherSeriesSet := make(map[string]struct{})
		for _, s := range allSeriesNames[seriesLimit-2:] {
			otherSeriesSet[s] = struct{}{}
		}

		newCounts := make([]map[string]int, len(b.counts))
		for i, binCounts := range b.counts {
			newBinCounts := make(map[string]int)
			for seriesName, count := range binCounts {
				if _, isOther := otherSeriesSet[seriesName]; isOther {
					newBinCounts[otherSeriesName] += count
				} else {
					newBinCounts[seriesName] = count
				}
			}
			newCounts[i] = newBinCounts
		}
		b.counts = newCounts

		slices.Sort(topSeries)
		seriesNames = append(topSeries, otherSeriesName)

		b.series = make(map[string]struct{})
		for _, name := range seriesNames {
			b.series[name] = struct{}{}
		}
	} else {
		seriesNames = slices.Collect(maps.Keys(b.series))
		slices.Sort(seriesNames)
	}

	var styleFunc func(string, int) string
	if style == barCharStyle {
		styleFunc = func(name string, count int) string {
			idx := slices.Index(seriesNames, name)
			chr := barStyles[idx%len(barStyles)].char
			bar := strings.Repeat(chr, count)
			return bar
		}
	} else {
		styleFunc = func(name string, count int) string {
			idx := slices.Index(seriesNames, name)
			chr := blockBarChar
			bar := strings.Repeat(chr, count)
			return barStyles[idx%len(barStyles)].color + bar + barColorReset
		}
	}

	fmt.Printf("Total count: %d\n", b.total)
	fmt.Printf("Time range:  %s - %s\n", b.minTime.Format(time.RFC3339), b.maxTime.Format(time.RFC3339))
	if len(seriesNames) != 1 || seriesNames[0] != "" {
		fmt.Println("Legend:")
		for _, name := range seriesNames {
			fmt.Printf("    %s = %s\n", styleFunc(name, 1), name)
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
		for i := range barLen - assignedBarLen {
			seriesToIncrement := fractionSeriesNames[i%len(fractionSeriesNames)]
			barLens[seriesToIncrement]++
		}

		var barBuilder strings.Builder
		for _, seriesName := range seriesNames {
			if barPartLen, ok := barLens[seriesName]; ok && barPartLen > 0 {
				barBuilder.WriteString(styleFunc(seriesName, barPartLen))
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
