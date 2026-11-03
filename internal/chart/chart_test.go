package chart

import "testing"

func TestNiceMax(t *testing.T) {
	cases := []struct{ in, top, step float64 }{{0, 1, 1}, {7, 10, 2}, {13, 20, 5}, {230, 250, 50},
		{4783, 5000, 1000}, {98000, 100000, 20000}, {120000, 200000, 50000}, {241727, 250000, 50000}}
	for _, c := range cases {
		if top, step := NiceMax(c.in); top != c.top || step != c.step {
			t.Errorf("NiceMax(%v) = %v, %v, want %v, %v", c.in, top, step, c.top, c.step)
		}
	}
}

func TestStackedYears(t *testing.T) {
	years := []int{1999, 2000, 2001}
	c := StackedYears(years, []Series{
		{Name: "a", Class: "s1", Values: []float64{10, 0, 5}},
		{Name: "b", Class: "s2", Values: []float64{5, 10, 0}},
	}, func(i int) string { return "" })
	if len(c.Columns) != 3 {
		t.Fatalf("columns = %d", len(c.Columns))
	}
	// Zero values draw no segment.
	if n := len(c.Columns[1].Segments); n != 1 {
		t.Errorf("year 2000 segments = %d, want 1", n)
	}
	// The tallest stack (15) reaches 15/20 of the plot height.
	col := c.Columns[0]
	top := col.Segments[1].Y
	want := c.Bottom - (c.Bottom-c.Top)*15/20
	if top < want-0.01 || top > want+0.01 {
		t.Errorf("stack top = %v, want %v", top, want)
	}
	if len(c.XTicks) != 1 || c.XTicks[0].Label != "2000" {
		t.Errorf("x ticks = %v", c.XTicks)
	}
}

func TestCount(t *testing.T) {
	if got := Count(1234567); got != "1,234,567" {
		t.Errorf("Count = %q", got)
	}
}
