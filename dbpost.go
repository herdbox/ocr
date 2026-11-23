package main

import (
	"image"
	"math"
	"slices"
)

// DB post-processing settings of PP-OCRv5 det, axis-aligned only.
const (
	detThresh    = 0.3 // pixel is text above this probability
	detBoxThresh = 0.6 // a box needs this mean probability
	detMinSide   = 3   // boxes with a shorter side are noise
	detUnclip    = 1.5 // grows boxes back by area × detUnclip / perimeter
	mergeOverlap = 0.6 // boxes on one line overlap this much vertically
	mergeGap     = 1.0 // and are closer than this many line heights
)

// detBoxes turns the det model's probability map (w×h) into line boxes in map
// pixels.
func detBoxes(prob []float32, w, h int) []image.Rectangle {
	var boxes []image.Rectangle
	for _, b := range components(prob, w, h) {
		if min(b.Dx(), b.Dy()) < detMinSide || meanProb(prob, w, b) < detBoxThresh {
			continue
		}
		boxes = append(boxes, b)
	}
	boxes = mergeLines(boxes)
	bounds := image.Rect(0, 0, w, h)
	var out []image.Rectangle
	for i, b := range boxes {
		if u := keepOff(unclip(b), b, boxes, i).Intersect(bounds); !u.Empty() {
			out = append(out, u)
		}
	}
	return out
}

// keepOff trims grown box u of line b (boxes[i]) to halfway between b and the
// lines above and below it. Glyphs reach well past the shrunk text cores the
// map shows, and with tight line spacing a crop holding a neighbour's accents
// or descenders makes Tesseract read nothing or garbage.
func keepOff(u, b image.Rectangle, boxes []image.Rectangle, i int) image.Rectangle {
	for j, o := range boxes {
		if j == i || o.Max.X <= u.Min.X || o.Min.X >= u.Max.X {
			continue
		}
		if o.Min.Y >= b.Max.Y {
			u.Max.Y = min(u.Max.Y, (b.Max.Y+o.Min.Y+1)/2)
		} else if o.Max.Y <= b.Min.Y {
			u.Min.Y = max(u.Min.Y, (o.Max.Y+b.Min.Y+1)/2)
		}
	}
	return u
}

// components returns the bounding box of each 8-connected region above
// detThresh, using two-pass labelling with union-find.
func components(prob []float32, w, h int) []image.Rectangle {
	labels := make([]int32, w*h)
	parent := []int32{0}
	var find func(int32) int32
	find = func(a int32) int32 {
		for parent[a] != a {
			parent[a] = parent[parent[a]]
			a = parent[a]
		}
		return a
	}
	union := func(a, b int32) int32 {
		ra, rb := find(a), find(b)
		if ra == rb {
			return ra
		}
		if ra < rb {
			parent[rb] = ra
			return ra
		}
		parent[ra] = rb
		return rb
	}
	for y := range h {
		for x := range w {
			i := y*w + x
			if prob[i] <= detThresh {
				continue
			}
			var l int32
			for _, n := range [4][2]int{{-1, 0}, {-1, -1}, {0, -1}, {1, -1}} {
				nx, ny := x+n[0], y+n[1]
				if nx < 0 || ny < 0 || nx >= w {
					continue
				}
				if nl := labels[ny*w+nx]; nl != 0 {
					if l == 0 {
						l = find(nl)
					} else {
						l = union(l, nl)
					}
				}
			}
			if l == 0 {
				l = int32(len(parent))
				parent = append(parent, l)
			}
			labels[i] = l
		}
	}
	boxes := map[int32]image.Rectangle{}
	var order []int32
	for y := range h {
		for x := range w {
			l := labels[y*w+x]
			if l == 0 {
				continue
			}
			r := find(l)
			px := image.Rect(x, y, x+1, y+1)
			if b, ok := boxes[r]; ok {
				boxes[r] = b.Union(px)
			} else {
				boxes[r] = px
				order = append(order, r)
			}
		}
	}
	out := make([]image.Rectangle, 0, len(order))
	for _, r := range order {
		out = append(out, boxes[r])
	}
	return out
}

func meanProb(prob []float32, w int, b image.Rectangle) float64 {
	var sum float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			sum += float64(prob[y*w+x])
		}
	}
	return sum / float64(b.Dx()*b.Dy())
}

// mergeLines joins boxes that sit on one line: vertical overlap of at least
// mergeOverlap of the shorter box and a horizontal gap under mergeGap times
// the taller box's height.
func mergeLines(boxes []image.Rectangle) []image.Rectangle {
	boxes = slices.Clone(boxes)
	for changed := true; changed; {
		changed = false
		for i := 0; i < len(boxes); i++ {
			for j := i + 1; j < len(boxes); j++ {
				if sameLine(boxes[i], boxes[j]) {
					boxes[i] = boxes[i].Union(boxes[j])
					boxes = slices.Delete(boxes, j, j+1)
					changed = true
					j = i
				}
			}
		}
	}
	slices.SortFunc(boxes, func(a, b image.Rectangle) int {
		if a.Min.Y != b.Min.Y {
			return a.Min.Y - b.Min.Y
		}
		return a.Min.X - b.Min.X
	})
	return boxes
}

func sameLine(a, b image.Rectangle) bool {
	overlap := min(a.Max.Y, b.Max.Y) - max(a.Min.Y, b.Min.Y)
	if float64(overlap) < mergeOverlap*float64(min(a.Dy(), b.Dy())) {
		return false
	}
	gap := max(a.Min.X, b.Min.X) - min(a.Max.X, b.Max.X)
	return float64(gap) < mergeGap*float64(max(a.Dy(), b.Dy()))
}

// unclip grows a box by area × detUnclip / perimeter on each side, as DB's
// unclip does for a rectangle.
func unclip(b image.Rectangle) image.Rectangle {
	w, h := float64(b.Dx()), float64(b.Dy())
	d := int(math.Round(w * h * detUnclip / (2 * (w + h))))
	return image.Rect(b.Min.X-d, b.Min.Y-d, b.Max.X+d, b.Max.Y+d)
}
