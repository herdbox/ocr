//go:build e2e

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// End-to-end tests need a package built by build/build.sh
// (HERDBOX_OCR_PKG_DIR) and the fixtures (HERDBOX_OCR_FIXTURES, default
// testdata).

var (
	e2eOnce    sync.Once
	e2eDet     *detector
	e2eReaders []*reader
	e2eErr     error
	e2eLines   chan line
)

func engine(t *testing.T) (*detector, chan line) {
	t.Helper()
	dir := os.Getenv("HERDBOX_OCR_PKG_DIR")
	if dir == "" {
		t.Fatal("HERDBOX_OCR_PKG_DIR not set")
	}
	e2eOnce.Do(func() {
		e2eDet, e2eReaders, e2eErr = load(dir)
		if e2eErr != nil {
			return
		}
		e2eLines = make(chan line)
		for _, r := range e2eReaders {
			go func() {
				for l := range e2eLines {
					readLine(r, l)
				}
			}()
		}
	})
	if e2eErr != nil {
		t.Fatal(e2eErr)
	}
	return e2eDet, e2eLines
}

func fixtures() string {
	if d := os.Getenv("HERDBOX_OCR_FIXTURES"); d != "" {
		return d
	}
	return "testdata"
}

func ocrFile(t *testing.T, path string) []item {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return ocrPNG(t, body)
}

func ocrPNG(t *testing.T, body []byte) []item {
	t.Helper()
	det, lines := engine(t)
	a := recognize(det, lines, request{ID: 1, Op: "ocr", Region: [4]int{0, 0, 1 << 20, 1 << 20}}, body)
	if a.Error != "" || a.Items == nil {
		t.Fatalf("%s: %s", a.Error, a.Message)
	}
	return *a.Items
}

func TestFROCR01_SettingsFixture(t *testing.T) {
	start := time.Now()
	items := ocrFile(t, filepath.Join(fixtures(), "settings.png"))
	t.Logf("%d items in %v (cold)", len(items), time.Since(start))
	start = time.Now()
	ocrFile(t, filepath.Join(fixtures(), "settings.png"))
	t.Logf("warm %v", time.Since(start))
	if b, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "VmHWM:") {
				t.Logf("peak RSS %s", strings.TrimSpace(strings.TrimPrefix(l, "VmHWM:")))
			}
		}
	}
	want, err := os.ReadFile(filepath.Join(fixtures(), "settings.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.Text)
		b := it.Bounds
		if b[0] < 0 || b[1] < 0 || b[2] > 1080 || b[3] > 2400 || b[0] >= b[2] || b[1] >= b[3] || it.Confidence < 0 || it.Confidence > 1 {
			t.Errorf("bad item %+v", it)
		}
	}
	for _, w := range strings.Split(strings.TrimSpace(string(want)), "\n") {
		if !slices.Contains(got, w) {
			t.Errorf("missing %q in %q", w, got)
		}
	}
}

func TestFROCR01_EmptyScreen(t *testing.T) {
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1080, 2400))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if items := ocrPNG(t, b.Bytes()); len(items) != 0 {
		t.Errorf("%+v", items)
	}
}

func TestFROCR01_RegionOnly(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(fixtures(), "settings.png"))
	if err != nil {
		t.Fatal(err)
	}
	det, lines := engine(t)
	full := ocrPNG(t, body)
	if len(full) == 0 {
		t.Fatal("no text")
	}
	b := full[len(full)/2].Bounds
	region := [4]int{0, b[1] - 10, 1080, b[3] + 10}
	a := recognize(det, lines, request{ID: 2, Op: "ocr", Region: region}, body)
	if a.Items == nil {
		t.Fatal(a.Error)
	}
	for _, it := range *a.Items {
		if it.Bounds[1] < region[1] || it.Bounds[3] > region[3] {
			t.Errorf("outside region: %+v", it)
		}
	}
	if len(*a.Items) == 0 {
		t.Errorf("nothing in region %v", region)
	}
}

// gtLine is one expected line of a test screen (testdata/*/gt.json).
type gtLine struct {
	Text string `json:"text"`
}

// TestOCR_VietnameseSet reads every screen under testdata/synthetic and
// testdata/real; ≥ 95 % of expected lines must come back exactly (SC-001a).
func TestOCR_VietnameseSet(t *testing.T) {
	var total, exact int
	var misses []string
	for _, set := range []string{"synthetic", "real"} {
		gtFile := filepath.Join("testdata", set, "gt.json")
		raw, err := os.ReadFile(gtFile)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var screens map[string][]gtLine
		if err := json.Unmarshal(raw, &screens); err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(screens))
		for n := range screens {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, name := range names {
			items := ocrFile(t, filepath.Join("testdata", set, name))
			got := map[string]int{}
			for _, it := range items {
				got[it.Text]++
			}
			for _, l := range screens[name] {
				total++
				if got[l.Text] > 0 {
					got[l.Text]--
					exact++
				} else {
					misses = append(misses, fmt.Sprintf("%s/%s: %q", set, name, l.Text))
				}
			}
		}
	}
	if total == 0 {
		t.Fatal("no test screens")
	}
	pct := float64(exact) * 100 / float64(total)
	t.Logf("exact lines: %d/%d (%.1f%%)", exact, total, pct)
	for _, m := range misses {
		t.Logf("miss %s", m)
	}
	if pct < 95 {
		t.Errorf("%.1f%% < 95%%", pct)
	}
}
