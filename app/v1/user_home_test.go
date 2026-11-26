package v1

import (
	"strings"
	"testing"
)

func TestHomeLadderStartCentresThePlayer(t *testing.T) {
	cases := []struct {
		name                     string
		index, size, count, want int64
	}{
		{"middle", 100, 11, 5000, 95},
		{"top", 2, 11, 5000, 0},
		{"bottom", 4998, 11, 5000, 4989},
		{"unranked, below the last", 5000, 11, 5000, 4989},
		{"board smaller than the ladder", 3, 11, 5, 0},
	}
	for _, c := range cases {
		if got := homeLadderStart(c.index, c.size, c.count); got != c.want {
			t.Errorf("%s: homeLadderStart(%d, %d, %d) = %d, want %d",
				c.name, c.index, c.size, c.count, got, c.want)
		}
	}
}

func TestHomeUTCOffsetTakesWholeMinutesWithinADay(t *testing.T) {
	cases := map[string]int64{
		"":     0,
		"300":  18000,
		"-480": -28800,
		"840":  50400,
		"841":  0, // past +14 hours
		"-841": 0,
		"x":    0,
		"90.5": 0,
	}
	for minutes, want := range cases {
		if got := homeUTCOffset(minutes); got != want {
			t.Errorf("homeUTCOffset(%q) = %d, want %d", minutes, got, want)
		}
	}
}

func TestHomeScoreColumnsNameTheTableOnly(t *testing.T) {
	columns := homeScoreColumns("scores_relax")
	for _, want := range []string{"scores_relax.id,", "scores_relax.userid", "beatmaps.beatmap_id"} {
		if !strings.Contains(columns, want) {
			t.Errorf("homeScoreColumns lacks %q: %s", want, columns)
		}
	}
	if strings.Contains(columns, "{t}") || strings.Contains(columns, "beatmapscores") {
		t.Errorf("homeScoreColumns: %s", columns)
	}
}
