package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestStringToTimeEpochFormats(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		format   string
		expected time.Time
	}{
		{
			name:     "unix seconds",
			input:    "1136239445",
			format:   "unix",
			expected: time.Unix(1136239445, 0).UTC(),
		},
		{
			name:     "unix milliseconds",
			input:    "1136239445000",
			format:   "unix-milli",
			expected: time.UnixMilli(1136239445000).UTC(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stringToTime(tt.input, tt.format)
			if err != nil {
				t.Fatalf("stringToTime returned error: %v", err)
			}

			if !got.Equal(tt.expected) {
				t.Fatalf("unexpected time\nexpected: %v\n     got: %v", tt.expected, got)
			}
		})
	}
}

func TestStringToTimeAutoDetectRFC3339(t *testing.T) {
	input := "2006-01-02T15:04:05Z"
	expected := time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC)

	got, err := stringToTime(input, "")
	if err != nil {
		t.Fatalf("stringToTime returned error: %v", err)
	}

	if !got.Equal(expected) {
		t.Fatalf("unexpected time\nexpected: %v\n     got: %v", expected, got)
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
