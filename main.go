package main

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/haccht/timeconv"
	"github.com/spf13/pflag"
)

const layoutExamples = timeconv.LayoutExamples

type barStyle struct {
	char string
	ansi string
}

var barStyles = []barStyle{
	{"|", "\x1b[1;34m"},
	{"#", "\x1b[1;31m"},
	{"=", "\x1b[1;33m"},
	{"x", "\x1b[1;35m"},
	{"+", "\x1b[1;32m"},
	{"*", "\x1b[1;36m"},
	{"o", "\x1b[37m"},
	{"~", "\x1b[96m"},
	{"^", "\x1b[95m"},
	{"%", "\x1b[92m"},
	{"@", "\x1b[93m"},
	{"/", "\x1b[94m"},
	{"\\", "\x1b[91m"},
	{":", "\x1b[36m"},
	{"?", "\x1b[32m"},
	{"!", "\x1b[35m"},
}

const blockBarChar = "▇"
const barColorReset = "\x1b[0m"
const otherSeriesName = "(Other)"

type barStyleOption int

const (
	barCharStyle barStyleOption = iota
	barColorStyle
)

type styleRenderer struct {
	mode   barStyleOption
	styles map[string]barStyle
}

type displayData struct {
	counts      []map[string]int
	seriesNames []string
	totals      map[string]int
}

type options struct {
	format    string
	interval  time.Duration
	barlen    int
	limit     int
	location  locationValue
	color     string
	separator string
	strict    bool
	verbose   bool
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

	pflag.StringVarP(&opts.format, "format", "f", "auto", "Input time format")
	pflag.DurationVarP(&opts.interval, "interval", "i", 5*time.Minute, "Bin width as duration (e.g. 30s, 1m, 1h)")
	pflag.IntVarP(&opts.barlen, "barlength", "b", 80, "Length of the longest bar")
	pflag.IntVarP(&opts.limit, "limit", "L", len(barStyles), "Maximum number of series")
	pflag.VarP(&opts.location, "location", "l", "Timezone location (e.g., UTC, Asia/Tokyo)")
	pflag.StringVar(&opts.color, "color", "auto", "Markup bar color [never|always|auto]")
	pflag.StringVarP(&opts.separator, "separator", "F", " ", "Field separator between timestamp and series")
	pflag.BoolVar(&opts.strict, "strict", false, "Stop at the first line with an invalid timestamp")
	pflag.BoolVarP(&opts.verbose, "verbose", "v", false, "Report each skipped input line")

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
	if err := validateOptions(&opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

func validateOptions(opts *options) error {
	if opts.interval <= 0 {
		return fmt.Errorf("interval must be greater than zero")
	}
	if opts.barlen <= 0 {
		return fmt.Errorf("barlength must be greater than zero")
	}
	if opts.limit <= 0 {
		return fmt.Errorf("limit must be greater than zero")
	}
	if _, err := resolveBarStyle(opts.color, 0, false); err != nil {
		return err
	}
	return nil
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

func parseLeadingTime(s, format, separator string) (time.Time, string) {
	t, series, _ := parseLeadingTimeInLocation(s, format, separator, time.UTC)
	return t, series
}

func parseLeadingTimeInLocation(s, format, separator string, location *time.Location) (time.Time, string, error) {
	if separator == "" {
		separator = " "
	}
	if strings.EqualFold(format, "auto") {
		format = ""
	}

	fields := strings.Split(s, separator)

	// Try the longest prefix first so an auto-detected DateOnly layout does not
	// consume the date portion of a DateTime timestamp.
	for i := len(fields) - 1; i >= 0; i-- {
		part1 := strings.Join(fields[:i+1], separator)
		part2 := strings.Join(fields[i+1:], separator)

		t, err := timeconv.ParseInLocation(part1, format, location)
		if err == nil {
			return t, part2, nil
		}
	}

	return time.Time{}, s, fmt.Errorf("unknown timestamp format")
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

func binIndex(t, base time.Time, size time.Duration) int {
	delta := t.Sub(base)
	idx := delta / size
	if delta < 0 && delta%size != 0 {
		idx--
	}
	return int(idx)
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

	idx := binIndex(t, b.base, b.size)
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

func supportsColorOutput(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}

	info, err := f.Stat()
	if err != nil {
		return false
	}

	return supportsColorTerminal(os.Getenv("TERM"), os.Getenv("NO_COLOR"), info.Mode()&os.ModeCharDevice != 0)
}

func supportsColorTerminal(term, noColor string, isTTY bool) bool {
	if !isTTY {
		return false
	}
	if noColor != "" {
		return false
	}
	if term == "" || term == "dumb" {
		return false
	}
	return true
}

func resolveBarStyle(color string, seriesCount int, colorCapable bool) (barStyleOption, error) {
	switch color {
	case "always":
		return barColorStyle, nil
	case "never":
		return barCharStyle, nil
	case "auto":
		if colorCapable && seriesCount > 1 {
			return barColorStyle, nil
		}
		return barCharStyle, nil
	default:
		return 0, fmt.Errorf("invalid color %q", color)
	}
}

func newStyleRenderer(mode barStyleOption, seriesNames []string) styleRenderer {
	styles := make(map[string]barStyle, len(seriesNames))
	for idx, name := range seriesNames {
		styles[name] = barStyles[idx%len(barStyles)]
	}
	return styleRenderer{
		mode:   mode,
		styles: styles,
	}
}

func (r styleRenderer) swatch(name string) string {
	return r.bar(name, 1)
}

func (r styleRenderer) bar(name string, count int) string {
	style, ok := r.styles[name]
	if !ok || count <= 0 {
		return ""
	}
	if r.mode == barCharStyle {
		return strings.Repeat(style.char, count)
	}
	return style.ansi + strings.Repeat(blockBarChar, count) + barColorReset
}

func renderSummary(w io.Writer, b *bins) {
	fmt.Fprintf(w, "Total count: %d\n", b.total)
	fmt.Fprintf(w, "Time range:  %s - %s\n", b.minTime.Format(time.RFC3339), b.maxTime.Format(time.RFC3339))
}

func totalCount(seriesCounts map[string]int) int {
	total := 0
	for _, count := range seriesCounts {
		total += count
	}
	return total
}

func seriesTotals(counts []map[string]int) map[string]int {
	totals := make(map[string]int)
	for _, binCounts := range counts {
		for seriesName, count := range binCounts {
			totals[seriesName] += count
		}
	}
	return totals
}

func sortedSeriesNames(series map[string]struct{}) []string {
	names := slices.Collect(maps.Keys(series))
	slices.Sort(names)
	return names
}

func limitSeries(counts []map[string]int, series map[string]struct{}, limit int) displayData {
	names := sortedSeriesNames(series)
	totals := seriesTotals(counts)
	if len(names) <= limit {
		return displayData{
			counts:      counts,
			seriesNames: names,
			totals:      totals,
		}
	}

	rankedNames := append([]string(nil), names...)
	slices.SortFunc(rankedNames, func(a, b string) int {
		if totals[b] != totals[a] {
			return totals[b] - totals[a]
		}
		return strings.Compare(a, b)
	})

	topSeries := append([]string(nil), rankedNames[:limit-1]...)
	slices.Sort(topSeries)

	otherSeriesSet := make(map[string]struct{}, len(rankedNames)-len(topSeries))
	for _, name := range rankedNames[limit-1:] {
		otherSeriesSet[name] = struct{}{}
	}

	limitedCounts := make([]map[string]int, len(counts))
	for i, binCounts := range counts {
		limitedBinCounts := make(map[string]int, len(binCounts))
		for seriesName, count := range binCounts {
			if _, ok := otherSeriesSet[seriesName]; ok {
				limitedBinCounts[otherSeriesName] += count
				continue
			}
			limitedBinCounts[seriesName] = count
		}
		limitedCounts[i] = limitedBinCounts
	}

	seriesNames := append(topSeries, otherSeriesName)
	return displayData{
		counts:      limitedCounts,
		seriesNames: seriesNames,
		totals:      seriesTotals(limitedCounts),
	}
}

func renderLegend(w io.Writer, renderer styleRenderer, seriesNames []string, totals map[string]int) {
	if len(seriesNames) == 1 && seriesNames[0] == "" {
		return
	}

	fmt.Fprintln(w, "Legend:")
	for _, name := range seriesNames {
		fmt.Fprintf(w, "    %s = %s (%d)\n", renderer.swatch(name), name, totals[name])
	}
}

func barLengths(seriesCounts map[string]int, seriesNames []string, maxBarLen, maxTotal int) map[string]int {
	totalInBin := totalCount(seriesCounts)
	if totalInBin == 0 || maxTotal == 0 {
		return nil
	}

	scaledBarLen := maxBarLen * totalInBin / maxTotal
	lengths := make(map[string]int, len(seriesCounts))
	fractions := make(map[string]int, len(seriesCounts))
	assigned := 0

	for _, seriesName := range seriesNames {
		count, ok := seriesCounts[seriesName]
		if !ok {
			continue
		}

		value := count * scaledBarLen * 100 / totalInBin
		lengths[seriesName] = value / 100
		fractions[seriesName] = value % 100
		assigned += lengths[seriesName]
	}

	fractionSeriesNames := slices.Collect(maps.Keys(fractions))
	slices.SortStableFunc(fractionSeriesNames, func(a, b string) int {
		return fractions[b] - fractions[a]
	})
	for i := range scaledBarLen - assigned {
		lengths[fractionSeriesNames[i%len(fractionSeriesNames)]]++
	}

	return lengths
}

func renderBar(renderer styleRenderer, seriesNames []string, lengths map[string]int) string {
	if len(lengths) == 0 {
		return ""
	}

	var barBuilder strings.Builder
	for _, seriesName := range seriesNames {
		if barPartLen := lengths[seriesName]; barPartLen > 0 {
			barBuilder.WriteString(renderer.bar(seriesName, barPartLen))
		}
	}
	return barBuilder.String()
}

func renderBins(w io.Writer, b *bins, counts []map[string]int, opts *options, renderer styleRenderer, seriesNames []string) error {
	maxTotalInBin := 0
	for _, seriesCounts := range counts {
		if total := totalCount(seriesCounts); total > maxTotalInBin {
			maxTotalInBin = total
		}
	}
	if maxTotalInBin == 0 {
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', tabwriter.AlignRight)
	for i, seriesCounts := range counts {
		t := b.base.Add(time.Duration(i) * b.size)
		totalInBin := totalCount(seriesCounts)
		if totalInBin == 0 {
			fmt.Fprintf(tw, "[\t%s\t]\t%6d\t  %s\n", t.Format(time.RFC3339), 0, "")
			continue
		}

		fmt.Fprintf(tw, "[\t%s\t]\t%6d\t  %s\n", t.Format(time.RFC3339), totalInBin, renderBar(renderer, seriesNames, barLengths(seriesCounts, seriesNames, opts.barlen, maxTotalInBin)))
	}

	return tw.Flush()
}

func renderHistogram(w io.Writer, b *bins, opts *options) error {
	if b.total == 0 {
		fmt.Fprintln(w, "Total count: 0")
		return nil
	}

	style, err := resolveBarStyle(opts.color, len(b.series), supportsColorOutput(w))
	if err != nil {
		return err
	}

	display := limitSeries(b.counts, b.series, opts.limit)
	renderer := newStyleRenderer(style, display.seriesNames)
	renderSummary(w, b)
	renderLegend(w, renderer, display.seriesNames, display.totals)
	fmt.Fprintln(w)

	return renderBins(w, b, display.counts, opts, renderer, display.seriesNames)
}

func collectBins(reader io.Reader, diagnostics io.Writer, opts *options) (*bins, error) {
	b := newBins(opts.interval)
	scanner := bufio.NewScanner(reader)
	lineNumber := 0
	skipped := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		t, seriesName, err := parseLeadingTimeInLocation(line, opts.format, opts.separator, opts.location.Location)
		if err != nil {
			if opts.strict {
				return nil, fmt.Errorf("line %d: %w: %q", lineNumber, err, line)
			}
			skipped++
			if opts.verbose {
				fmt.Fprintf(diagnostics, "skipped line %d: %v: %q\n", lineNumber, err, line)
			}
			continue
		}

		t = t.In(opts.location.Location)
		if t.Year() == 0 {
			t = t.AddDate(time.Now().Year(), 0, 0)
		}

		b.add(t, seriesName)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if skipped > 0 {
		fmt.Fprintf(diagnostics, "Skipped lines: %d\n", skipped)
	}

	return b, nil
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

	b, err := collectBins(reader, os.Stderr, opts)
	if err != nil {
		return err
	}

	return renderHistogram(os.Stdout, b, opts)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
