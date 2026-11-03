package chart

import "sort"

// Item is something to place in a treemap, sized by Value.
type Item struct {
	Name, URL, Tip string
	Value          float64
}

// Tile is an item placed in the treemap. Position and size are percentages
// of the treemap's width and height.
type Tile struct {
	Item
	X, Y, W, H float64
	Share      float64 // per cent of the treemap total
	Label      bool    // big enough to carry a text label
}

// Treemap lays out items with a positive Value using the squarified
// algorithm (Bruls, Huizing and van Wijk) in a box of the given aspect ratio
// (width / height). Copied from spendingexplorer's internal/explore.
func Treemap(items []Item, aspect float64) []Tile {
	var in []Item
	var total float64
	for _, it := range items {
		if it.Value > 0 {
			in = append(in, it)
			total += it.Value
		}
	}
	if total == 0 {
		return nil
	}
	sort.SliceStable(in, func(i, j int) bool { return in[i].Value > in[j].Value })

	// Work in a w x h box with area = w*h, then convert to percentages.
	w, h := 100*aspect, 100.0
	scale := w * h / total
	areas := make([]float64, len(in))
	for i, it := range in {
		areas[i] = it.Value * scale
	}

	tiles := make([]Tile, 0, len(in))
	x, y := 0.0, 0.0
	for i := 0; i < len(in); {
		side := min(w, h)
		end := i + 1
		for end < len(in) && worst(areas[i:end+1], side) <= worst(areas[i:end], side) {
			end++
		}
		var sum float64
		for _, a := range areas[i:end] {
			sum += a
		}
		if w >= h {
			// Column on the left, items stacked top to bottom.
			colW := sum / h
			cy := y
			for k := i; k < end; k++ {
				th := areas[k] / colW
				tiles = append(tiles, tile(in[k], x, cy, colW, th, aspect, total))
				cy += th
			}
			x += colW
			w -= colW
		} else {
			// Row along the top, items left to right.
			rowH := sum / w
			cx := x
			for k := i; k < end; k++ {
				tw := areas[k] / rowH
				tiles = append(tiles, tile(in[k], cx, y, tw, rowH, aspect, total))
				cx += tw
			}
			y += rowH
			h -= rowH
		}
		i = end
	}
	return tiles
}

func tile(it Item, x, y, w, h, aspect, total float64) Tile {
	t := Tile{
		Item:  it,
		X:     x / aspect,
		Y:     y,
		W:     w / aspect,
		H:     h,
		Share: 100 * it.Value / total,
	}
	t.Label = t.W >= 9 && t.H >= 14
	return t
}

// worst is the highest aspect ratio in a row of areas laid along side s.
func worst(row []float64, s float64) float64 {
	var sum, lo, hi float64
	lo = row[0]
	for _, a := range row {
		sum += a
		lo = min(lo, a)
		hi = max(hi, a)
	}
	s2, sum2 := s*s, sum*sum
	return max(s2*hi/sum2, sum2/(s2*lo))
}
