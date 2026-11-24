package aql

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDuration(t *testing.T) {
	const day = 24 * time.Hour

	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"2h", 2 * time.Hour},
		{"30m", 30 * time.Minute},
		{"2h 30m", 2*time.Hour + 30*time.Minute},
		{"2h30m", 2*time.Hour + 30*time.Minute},
		{"30m 2h", 2*time.Hour + 30*time.Minute},
		{"1w\t2d", 9 * day},
		{"2 h", 2 * time.Hour},
		{"  2h  ", 2 * time.Hour},
		{"2H", 2 * time.Hour},
		{"1m", time.Minute},

		{"2ч 30м", 2*time.Hour + 30*time.Minute},
		{"2Ч30М", 2*time.Hour + 30*time.Minute},
		{"45мин", 45 * time.Minute},
		{"3д", 3 * day},
		{"1нед", 7 * day},
		{"1н", 7 * day},

		{"1.5h", 90 * time.Minute},
		{"0.25d", 6 * time.Hour},
		{"0.1h", 6 * time.Minute},

		{"-7d", -7 * day},
		{"+1w 2d", 9 * day},
		{"-1w 2d", -9 * day},
		{"- 7d", -7 * day},
		{"-0m", 0},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := parseDuration(tc.raw)
			require.NoError(t, err)
			assert.Equal(t, Duration{Fixed: tc.want}, got)
		})
	}
}

func TestParseDurationInvalid(t *testing.T) {
	cases := []struct {
		raw     string
		wantErr error
	}{
		{"", nil},
		{"   ", nil},
		{"-", nil},
		{"+", nil},
		{"2", nil},
		{"h", nil},
		{"1.h", nil},
		{".5h", nil},
		{"1,5h", nil},
		{"2h-30m", nil},
		{"2h, 30m", nil},
		{"--7d", nil},
		{"+-7d", nil},

		{"2x", ErrDurationUnit},
		{"2hours", ErrDurationUnit},
		{"2мин30с", ErrDurationUnit},

		{"100000w", ErrDurationRange},
		{"99999999999999999999h", ErrDurationRange},
		{"2000000h 2000000h", ErrDurationRange},
		{"99999999999M", ErrDurationRange},

		{"1.5M", ErrDurationFraction},
		{"0.5y", ErrDurationFraction},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			_, err := parseDuration(tc.raw)
			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestParseDurationCalendar(t *testing.T) {
	cases := []struct {
		raw  string
		want Duration
	}{
		{"1M", Duration{Months: 1}},
		{"1y", Duration{Months: 12}},
		{"1Y", Duration{Months: 12}},
		{"1г", Duration{Months: 12}},
		{"2мес", Duration{Months: 2}},
		{"2МЕС", Duration{Months: 2}},
		{"1y 2M", Duration{Months: 14}},
		{"1w 1M", Duration{Months: 1, Fixed: 7 * 24 * time.Hour}},
		{"-1M", Duration{Months: -1}},
		{"-1M 2d", Duration{Months: -1, Fixed: -2 * 24 * time.Hour}},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := parseDuration(tc.raw)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
