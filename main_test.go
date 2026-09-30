package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestParseLeadingTimeAutoDetectsUnixMilliseconds(t *testing.T) {
	want := time.Unix(1698292629, 955000000)
	got, series := parseLeadingTime("1698292629955 api", "", " ")
	if !got.Equal(want) {
		t.Fatalf("unexpected time\nexpected: %v\n     got: %v", want, got)
	}
	if series != "api" {
		t.Fatalf("series = %q, want %q", series, "api")
	}
}

func TestParseLeadingTimeWithSeparator(t *testing.T) {
	input := "2006-01-02T15:04:05Z,series-name,detail"

	wantTime := time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC)
	wantSeries := "series-name,detail"

	gotTime, gotSeries := parseLeadingTime(input, "rfc3339", ",")

	if !gotTime.Equal(wantTime) {
		t.Fatalf("unexpected time\\nexpected: %v\\n     got: %v", wantTime, gotTime)
	}

	if gotSeries != wantSeries {
		t.Fatalf("unexpected series\\nexpected: %q\\n     got: %q", wantSeries, gotSeries)
	}
}

func TestParseLeadingTimeAutoDetectsLongestTimestamp(t *testing.T) {
	input := "2026-09-29 12:34:56 api"
	wantTime := time.Date(2026, time.September, 29, 12, 34, 56, 0, time.UTC)

	gotTime, gotSeries := parseLeadingTime(input, "", " ")

	if !gotTime.Equal(wantTime) {
		t.Fatalf("unexpected time\nexpected: %v\n     got: %v", wantTime, gotTime)
	}
	if gotSeries != "api" {
		t.Fatalf("series = %q, want %q", gotSeries, "api")
	}
}

func TestParseLeadingTimeUsesLocationForZoneLessTimestamp(t *testing.T) {
	location, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("time.LoadLocation returned error: %v", err)
	}
	want := time.Date(2026, time.September, 29, 12, 34, 56, 0, location)

	got, series, err := parseLeadingTimeInLocation("2026-09-29 12:34:56 api", "datetime", " ", location)
	if err != nil {
		t.Fatalf("parseLeadingTimeInLocation returned error: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("unexpected time\nexpected: %v\n     got: %v", want, got)
	}
	if series != "api" {
		t.Fatalf("series = %q, want %q", series, "api")
	}
}

func TestParseLeadingTimePreservesExplicitTimezone(t *testing.T) {
	location, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("time.LoadLocation returned error: %v", err)
	}
	want := time.Date(2026, time.September, 29, 12, 34, 56, 0, time.UTC)

	got, _, err := parseLeadingTimeInLocation("2026-09-29T12:34:56Z api", "rfc3339", " ", location)
	if err != nil {
		t.Fatalf("parseLeadingTimeInLocation returned error: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("unexpected time\nexpected: %v\n     got: %v", want, got)
	}
}

func TestValidateOptions(t *testing.T) {
	valid := options{interval: time.Minute, barlen: 80, limit: 16, color: "auto"}
	tests := []struct {
		name string
		edit func(*options)
	}{
		{name: "zero interval", edit: func(o *options) { o.interval = 0 }},
		{name: "negative interval", edit: func(o *options) { o.interval = -time.Minute }},
		{name: "zero bar length", edit: func(o *options) { o.barlen = 0 }},
		{name: "zero limit", edit: func(o *options) { o.limit = 0 }},
		{name: "invalid color", edit: func(o *options) { o.color = "sometimes" }},
	}

	if err := validateOptions(&valid); err != nil {
		t.Fatalf("valid options returned error: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := valid
			tt.edit(&opts)
			if err := validateOptions(&opts); err == nil {
				t.Fatal("validateOptions returned nil error")
			}
		})
	}
}

func TestCollectBinsReportsSkippedLines(t *testing.T) {
	opts := &options{
		interval: time.Minute,
		barlen:   80,
		limit:    16,
		location: locationValue{Location: time.UTC},
		color:    "never",
		verbose:  true,
	}
	input := strings.NewReader("2026-09-29T12:34:56Z api\ninvalid input\n")
	var diagnostics bytes.Buffer

	b, err := collectBins(input, &diagnostics, opts)
	if err != nil {
		t.Fatalf("collectBins returned error: %v", err)
	}
	if b.total != 1 {
		t.Fatalf("total = %d, want 1", b.total)
	}
	for _, want := range []string{"skipped line 2:", "Skipped lines: 1"} {
		if !strings.Contains(diagnostics.String(), want) {
			t.Fatalf("diagnostics missing %q\n%s", want, diagnostics.String())
		}
	}
}

func TestCollectBinsStrictRejectsInvalidLine(t *testing.T) {
	opts := &options{
		interval: time.Minute,
		barlen:   80,
		limit:    16,
		location: locationValue{Location: time.UTC},
		color:    "never",
		strict:   true,
	}

	_, err := collectBins(strings.NewReader("invalid input\n"), io.Discard, opts)
	if err == nil {
		t.Fatal("collectBins returned nil error")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("error missing line number: %v", err)
	}
}

func TestBinsAddEarlierTimeUsesPreviousBin(t *testing.T) {
	b := newBins(time.Minute)
	base := time.Date(2024, time.January, 2, 3, 4, 0, 0, time.UTC)

	b.add(base.Add(30*time.Second), "later")
	b.add(base.Add(-time.Second), "earlier")

	if !b.base.Equal(base.Add(-time.Minute)) {
		t.Fatalf("unexpected base\nexpected: %v\n     got: %v", base.Add(-time.Minute), b.base)
	}
	if got := totalCount(b.counts[0]); got != 1 {
		t.Fatalf("previous bin count = %d, want 1", got)
	}
	if got := totalCount(b.counts[1]); got != 1 {
		t.Fatalf("original bin count = %d, want 1", got)
	}
	if got := b.counts[0]["earlier"]; got != 1 {
		t.Fatalf("earlier series count = %d, want 1", got)
	}
	if got := b.counts[1]["later"]; got != 1 {
		t.Fatalf("later series count = %d, want 1", got)
	}
}

func TestRenderHistogramNeverUsesDistinctCharsPerSeries(t *testing.T) {
	b := newBins(time.Minute)
	base := time.Date(2024, time.January, 2, 3, 4, 0, 0, time.UTC)
	b.add(base, "seriesA")
	b.add(base, "seriesA")
	b.add(base, "seriesB")
	b.add(base, "seriesB")

	opts := &options{
		interval: time.Minute,
		barlen:   8,
		limit:    len(barStyles),
		color:    "never",
	}

	var out bytes.Buffer
	if err := renderHistogram(&out, b, opts); err != nil {
		t.Fatalf("renderHistogram returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Legend:",
		"    | = seriesA (2)",
		"    # = seriesB (2)",
		"  ||||####",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered output missing %q\n%s", want, got)
		}
	}
}

func TestRenderHistogramAlwaysUsesAnsiEmphasis(t *testing.T) {
	b := newBins(time.Minute)
	base := time.Date(2024, time.January, 2, 3, 4, 0, 0, time.UTC)
	b.add(base, "seriesA")
	b.add(base, "seriesB")

	opts := &options{
		interval: time.Minute,
		barlen:   4,
		limit:    len(barStyles),
		color:    "always",
	}

	var out bytes.Buffer
	if err := renderHistogram(&out, b, opts); err != nil {
		t.Fatalf("renderHistogram returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "\x1b[1;34m▇\x1b[0m = seriesA (1)") {
		t.Fatalf("rendered output missing bold blue legend entry\n%s", got)
	}
	if !strings.Contains(got, "\x1b[1;31m▇\x1b[0m = seriesB (1)") {
		t.Fatalf("rendered output missing red legend entry\n%s", got)
	}
	if !strings.Contains(got, "\x1b[1;34m▇▇\x1b[0m\x1b[1;31m▇▇\x1b[0m") {
		t.Fatalf("rendered output missing emphasized stacked bar\n%s", got)
	}
}

func TestRenderHistogramAggregatesOtherSeries(t *testing.T) {
	b := newBins(time.Minute)
	base := time.Date(2024, time.January, 2, 3, 4, 0, 0, time.UTC)
	b.add(base, "seriesA")
	b.add(base, "seriesA")
	b.add(base, "seriesB")
	b.add(base, "seriesC")

	opts := &options{
		interval: time.Minute,
		barlen:   8,
		limit:    2,
		color:    "never",
	}

	var out bytes.Buffer
	if err := renderHistogram(&out, b, opts); err != nil {
		t.Fatalf("renderHistogram returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"    | = seriesA (2)",
		"    # = (Other) (2)",
		"  ||||####",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered output missing %q\n%s", want, got)
		}
	}
}

func TestSupportsColorTerminal(t *testing.T) {
	tests := []struct {
		name    string
		term    string
		noColor string
		isTTY   bool
		want    bool
	}{
		{name: "tty with color term", term: "xterm-256color", isTTY: true, want: true},
		{name: "non tty", term: "xterm-256color", isTTY: false, want: false},
		{name: "no color env", term: "xterm-256color", noColor: "1", isTTY: true, want: false},
		{name: "empty term", term: "", isTTY: true, want: false},
		{name: "dumb term", term: "dumb", isTTY: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := supportsColorTerminal(tt.term, tt.noColor, tt.isTTY)
			if got != tt.want {
				t.Fatalf("supportsColorTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveBarStyleAutoFallsBackWithoutColorSupport(t *testing.T) {
	got, err := resolveBarStyle("auto", 2, false)
	if err != nil {
		t.Fatalf("resolveBarStyle returned error: %v", err)
	}
	if got != barCharStyle {
		t.Fatalf("resolveBarStyle() = %v, want %v", got, barCharStyle)
	}
}

func TestResolveBarStyleAutoUsesColorWhenSupported(t *testing.T) {
	got, err := resolveBarStyle("auto", 2, true)
	if err != nil {
		t.Fatalf("resolveBarStyle returned error: %v", err)
	}
	if got != barColorStyle {
		t.Fatalf("resolveBarStyle() = %v, want %v", got, barColorStyle)
	}
}
