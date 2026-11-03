package chart

import (
	"math"
	"testing"
)

func TestTreemapCoversBoxAndPreservesArea(t *testing.T) {
	items := []Item{{Name: "a", Value: 600}, {Name: "b", Value: 300}, {Name: "c", Value: 60}, {Name: "d", Value: 40}, {Name: "zero"}}
	tiles := Treemap(items, 2)
	if len(tiles) != 4 {
		t.Fatalf("got %d tiles, want 4 (zero left out)", len(tiles))
	}
	var area float64
	for _, tl := range tiles {
		if tl.X < -1e-9 || tl.Y < -1e-9 || tl.X+tl.W > 100+1e-6 || tl.Y+tl.H > 100+1e-6 {
			t.Errorf("tile %s out of bounds: %+v", tl.Name, tl)
		}
		area += tl.W * tl.H
		if got := tl.W * tl.H / 100; math.Abs(got-tl.Share) > 1e-6 {
			t.Errorf("tile %s area %.4f%%, want share %.4f%%", tl.Name, got, tl.Share)
		}
	}
	if math.Abs(area-10000) > 1e-6 {
		t.Errorf("tiles cover %.4f of 10000", area)
	}
}
