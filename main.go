package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	flags "github.com/jessevdk/go-flags"
	"golang.org/x/term"
)

const helpText = `Format Examples:
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
    Guess       (guess an appropriate format)

    Arbitrary formats are also supported. See https://pkg.go.dev/time as a reference.`

var layouts = map[string]string{
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

type options struct {
	Format       string `short:"f" long:"format" description:"Format for parsing the input time" default:"unix"`
	TimeInterval string `short:"i" long:"gap" description:"Time duration to aggregate" default:"5m"`
	Location     string `short:"z" long:"loc" description:"Override timezone" default:"UTC"`
	BarLength    int    `short:"l" long:"barlength" description:"Bar length" default:"60"`
	Help         bool   `short:"h" long:"help" description:"Show this help message"`
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

func stringToTime(s, format string) (time.Time, error) {
	if format == "guess" {
		return guessTime(s)
	}

	if layout, ok := layouts[format]; ok {
		return time.Parse(layout, s)
	}

	if scale, ok := epochLayouts[strings.ToLower(format)]; ok {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse epoch time: %s", s)
		}
		return time.UnixMicro(int64(v * float64(scale))), nil
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
	return time.Time{}, fmt.Errorf("Unknown format: %s", s)
}

func parseFlags() (*options, []string, error) {
	var opts options

	parser := flags.NewParser(&opts, flags.Default&^flags.HelpFlag)
	parser.Usage = "[Options]"
	args, err := parser.Parse()
	if err != nil {
		return nil, nil, err
	}

	if opts.Help {
		var message bytes.Buffer
		parser.WriteHelp(&message)
		fmt.Fprint(&message, "\n")
		fmt.Fprint(&message, helpText)
		fmt.Fprintln(os.Stdout, message.String())
		os.Exit(0)
	}

	opts.Format = strings.ToLower(opts.Format)
	return &opts, args, nil
}

type bins struct {
	base   time.Time
	size   time.Duration
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

func (b *bins) totalCount() int {
	var s int
	for _, v := range b.counts {
		s += v
	}
	return s
}

func (b *bins) maxCount() int {
	return slices.Max(b.counts)
}

func run() error {
	opts, args, err := parseFlags()
	if err != nil {
		return err
	}

	loc, err := time.LoadLocation(opts.Location)
	if err != nil {
		return err
	}

	gap, err := time.ParseDuration(opts.TimeInterval)
	if err != nil {
		return err
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

	h := newBins(gap)

	scanner := bufio.NewScanner(io.MultiReader(readers...))
	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		t, err := stringToTime(s, opts.Format)
		if err != nil {
			continue
		}

		h.add(t)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	total := h.totalCount()
	fmt.Printf("Total: %d items\n\n", total)
	if total == 0 {
		return nil
	}

	max := h.maxCount()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.AlignRight)
	for i, c := range h.counts {
		t := h.base.Add(time.Duration(i) * h.size)
		if t.Year() == 0 {
			t = t.AddDate(t.Year(), 0, 0)
		}

		bar := strings.Repeat("|", opts.BarLength*c/max)
		fmt.Fprintf(w, "[\t%s\t]\t%6d\t  %s\n", t.In(loc).Format(time.RFC3339), c, bar)
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
