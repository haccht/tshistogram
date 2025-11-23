package main

import (
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
