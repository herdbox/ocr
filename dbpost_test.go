package main

import (
	"image"
	"testing"
)

// probMap returns a w×h map with value v inside each box.
func probMap(w, h int, v float32, boxes ...image.Rectangle) []float32 {
	p := make([]float32, w*h)
	for _, b := range boxes {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				p[y*w+x] = v
			}
		}
	}
	return p
}

func TestFROCR01_DetPostThresholdAndScore(t *testing.T) {
	// 0.25 is below the pixel threshold; 0.5 passes it but not the box score.
	if got := detBoxes(probMap(100, 50, 0.25, image.Rect(10, 10, 60, 20)), 100, 50); len(got) != 0 {
		t.Errorf("below threshold: %v", got)
	}
	if got := detBoxes(probMap(100, 50, 0.5, image.Rect(10, 10, 60, 20)), 100, 50); len(got) != 0 {
		t.Errorf("low score: %v", got)
	}
	if got := detBoxes(probMap(100, 50, 0.9, image.Rect(10, 10, 60, 20)), 100, 50); len(got) != 1 {
		t.Errorf("one box: %v", got)
	}
}

func TestFROCR01_DetPostDropsSmall(t *testing.T) {
	if got := detBoxes(probMap(100, 50, 0.9, image.Rect(10, 10, 60, 12)), 100, 50); len(got) != 0 {
		t.Errorf("2 px tall: %v", got)
	}
}

func TestFROCR01_DetPostUnclip(t *testing.T) {
	// 50×10: d = 500 × 1.5 / 120 = 6.25 → 6.
	got := detBoxes(probMap(100, 50, 0.9, image.Rect(10, 20, 60, 30)), 100, 50)
	if len(got) != 1 || got[0] != image.Rect(4, 14, 66, 36) {
		t.Errorf("%v", got)
	}
	// Clamped to the map.
	got = detBoxes(probMap(100, 50, 0.9, image.Rect(0, 0, 50, 10)), 100, 50)
	if len(got) != 1 || got[0].Min != (image.Point{}) {
		t.Errorf("clamp: %v", got)
	}
}

func TestFROCR01_DetPostDiagonalConnected(t *testing.T) {
	p := probMap(40, 40, 0.9, image.Rect(5, 5, 15, 15), image.Rect(15, 15, 25, 25))
	if got := components(p, 40, 40); len(got) != 1 || got[0] != image.Rect(5, 5, 25, 25) {
		t.Errorf("%v", got)
	}
	// A U shape needs the union of two labels.
	u := probMap(40, 40, 0.9, image.Rect(5, 5, 8, 20), image.Rect(20, 5, 23, 20), image.Rect(5, 17, 23, 20))
	if got := components(u, 40, 40); len(got) != 1 || got[0] != image.Rect(5, 5, 23, 20) {
		t.Errorf("U: %v", got)
	}
}

func TestFROCR01_DetPostMergesWordsOnALine(t *testing.T) {
	// Two words 10 px tall, 6 px apart: one line.
	got := detBoxes(probMap(200, 60, 0.9, image.Rect(10, 20, 50, 30), image.Rect(56, 21, 90, 31)), 200, 60)
	if len(got) != 1 {
		t.Fatalf("words: %v", got)
	}
	// Far apart on one row (title and value): two boxes.
	got = detBoxes(probMap(200, 60, 0.9, image.Rect(10, 20, 50, 30), image.Rect(150, 20, 190, 30)), 200, 60)
	if len(got) != 2 {
		t.Fatalf("columns: %v", got)
	}
	// Stacked lines: two boxes, top first.
	got = detBoxes(probMap(200, 80, 0.9, image.Rect(10, 40, 90, 50), image.Rect(10, 20, 90, 30)), 200, 80)
	if len(got) != 2 || got[0].Min.Y > got[1].Min.Y {
		t.Fatalf("stacked: %v", got)
	}
}

func TestFROCR01_DetPostScaleToScreen(t *testing.T) {
	// A 1080×2400 screen maps to 448×960 (long side 960, sides multiple of 32).
	if w, h := detSize(1080, 2400); w != 448 || h != 960 {
		t.Errorf("%d×%d", w, h)
	}
	if w, h := detSize(20, 10); w != 960 || h != 480 {
		t.Errorf("small %d×%d", w, h)
	}
	if w, h := detSize(4000, 10); w != 960 || h != 32 {
		t.Errorf("thin %d×%d", w, h)
	}
}
