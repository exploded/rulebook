// Package chart lays out simple SVG charts. It computes geometry only; the
// templates draw it, and the CSS colours each series by its class.
package chart

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Frame is the shared coordinate space: a viewBox W x H, with the plot area
// inside the margins.
type Frame struct {
	W, H, Left, Right, Top, Bottom float64
	YTicks, XTicks                 []Tick
}

type Tick struct {
	Pos   float64
	Label string
}

type Segment struct {
	Y, H  float64
	Class string
}

type Column struct {
	X, W     float64
	Segments []Segment
	Tip      string
	HitY     float64 // top of the hover target: the plot's top edge
	HitH     float64
}

// Columns is a stacked column chart: one column per x, one segment per series.
type Columns struct {
	Frame
	Columns []Column
}

// Series names one stack layer and the CSS class that colours it.
type Series struct {
	Name, Class string
	Values      []float64
}

func newFrame() Frame {
	return Frame{W: 1000, H: 280, Left: 60, Right: 988, Top: 12, Bottom: 252}
}

// NiceMax rounds max up to 1, 2 or 5 times a power of ten, so the y axis
// ends on a round number, and returns the tick step.
func NiceMax(max float64) (top, step float64) {
	if max <= 0 {
		return 1, 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(max)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if m*mag >= max {
			top = m * mag
			break
		}
	}
	return top, top / 4
}

// Count formats n with thousands separators.
func Count(n float64) string {
	s := strconv.FormatInt(int64(math.Round(n)), 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func (f *Frame) yTicks(top, step float64) {
	for v := 0.0; v <= top+step/2; v += step {
		f.YTicks = append(f.YTicks, Tick{f.y(v, top), Count(v)})
	}
}

func (f Frame) y(v, top float64) float64 {
	return f.Bottom - (f.Bottom-f.Top)*v/top
}

// yearTicks labels every 20th year from the first year that is a multiple
// of 20.
func (f *Frame) yearTicks(years []int, x func(i int) float64) {
	for i, y := range years {
		if y%20 == 0 {
			f.XTicks = append(f.XTicks, Tick{x(i), strconv.Itoa(y)})
		}
	}
}

// StackedYears lays out one column per year, stacking the series bottom up.
// tip formats a column's tooltip.
func StackedYears(years []int, series []Series, tip func(i int) string) Columns {
	c := Columns{Frame: newFrame()}
	max := 0.0
	for i := range years {
		sum := 0.0
		for _, s := range series {
			sum += s.Values[i]
		}
		max = math.Max(max, sum)
	}
	top, step := NiceMax(max)
	c.yTicks(top, step)
	slot := (c.Right - c.Left) / float64(len(years))
	gap := math.Min(2, slot*0.25)
	x := func(i int) float64 { return c.Left + float64(i)*slot + slot/2 }
	c.yearTicks(years, x)
	for i := range years {
		col := Column{X: c.Left + float64(i)*slot + gap/2, W: slot - gap, Tip: tip(i), HitY: c.Top, HitH: c.Bottom - c.Top}
		base := 0.0
		for _, s := range series {
			v := s.Values[i]
			if v <= 0 {
				continue
			}
			y0, y1 := c.y(base, top), c.y(base+v, top)
			col.Segments = append(col.Segments, Segment{Y: y1, H: y0 - y1, Class: s.Class})
			base += v
		}
		c.Columns = append(c.Columns, col)
	}
	return c
}

// Area is a single series drawn as a line over a filled area, with one
// hover band per point.
type Area struct {
	Frame
	Line, Fill string // SVG path data
	Bands      []Column
}

// AreaYears lays out values by year.
func AreaYears(years []int, values []float64, tip func(i int) string) Area {
	a := Area{Frame: newFrame()}
	max := 0.0
	for _, v := range values {
		max = math.Max(max, v)
	}
	top, step := NiceMax(max)
	a.yTicks(top, step)
	slot := (a.Right - a.Left) / float64(max1(len(years)-1))
	x := func(i int) float64 { return a.Left + float64(i)*slot }
	a.yearTicks(years, x)
	var line strings.Builder
	for i, v := range values {
		cmd := "L"
		if i == 0 {
			cmd = "M"
		}
		fmt.Fprintf(&line, "%s%.1f %.1f ", cmd, x(i), a.y(v, top))
		a.Bands = append(a.Bands, Column{X: x(i) - slot/2, W: slot, Tip: tip(i), HitY: a.Top, HitH: a.Bottom - a.Top})
	}
	a.Line = strings.TrimSpace(line.String())
	if len(values) > 0 {
		a.Fill = fmt.Sprintf("%s L%.1f %.1f L%.1f %.1f Z", a.Line, x(len(values)-1), a.Bottom, x(0), a.Bottom)
	}
	return a
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
