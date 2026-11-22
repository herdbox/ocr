// Command herdbox-ocr reads text on phone screenshots for Herdbox nodes on
// Linux and Windows. It speaks a framed protocol on stdin/stdout: a request
// carries a PNG and a region, an answer the text lines found.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// protocol is the version of the stdin/stdout protocol.
const protocol = 1

// pad is added around each line before Tesseract reads it.
const pad = 4

type request struct {
	ID     int64  `json:"id"`
	Op     string `json:"op"`
	Region [4]int `json:"region"`
}

type item struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	Bounds     [4]int  `json:"bounds"`
}

type answer struct {
	ID      int64   `json:"id"`
	Items   *[]item `json:"items,omitempty"`
	OK      bool    `json:"ok,omitempty"`
	Error   string  `json:"error,omitempty"`
	Message string  `json:"message,omitempty"`
}

// line is one box for a worker to read; the answer goes to out.
type line struct {
	img  image.Image
	box  image.Rectangle
	out  *item
	done *sync.WaitGroup
}

func main() {
	os.Setenv("OMP_THREAD_LIMIT", "1")
	dir, err := packageDir()
	var det *detector
	var readers []*reader
	if err == nil {
		det, readers, err = load(dir)
	}
	if err != nil {
		fmt.Printf("herdbox-ocr-error %s\n", strings.ReplaceAll(err.Error(), "\n", " "))
		os.Exit(1)
	}
	fmt.Printf("herdbox-ocr %d %s\n", protocol, packageVersion(dir))
	os.Exit(serve(os.Stdin, os.Stdout, det, readers))
}

// packageDir is the folder holding the binary and the package files.
func packageDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

func packageVersion(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if v := strings.TrimSpace(string(b)); err == nil && v != "" {
		return v
	}
	return "0"
}

func load(dir string) (*detector, []*reader, error) {
	det, err := newDetector(dir, filepath.Join(dir, ortLibrary()))
	if err != nil {
		return nil, nil, err
	}
	n := min(runtime.NumCPU(), 4)
	readers := make([]*reader, n)
	for i := range readers {
		if readers[i], err = newReader(filepath.Join(dir, "tessdata")); err != nil {
			return nil, nil, err
		}
	}
	return det, readers, nil
}

// serve answers requests until stdin ends, then waits for the ones in flight.
func serve(in io.Reader, out io.Writer, det *detector, readers []*reader) int {
	w := bufio.NewWriter(out)
	var mu sync.Mutex
	send := func(a answer) {
		mu.Lock()
		defer mu.Unlock()
		if err := writeFrame(w, a, nil); err == nil {
			err = w.Flush()
		}
	}
	lines := make(chan line)
	var workers sync.WaitGroup
	for _, r := range readers {
		workers.Go(func() {
			for l := range lines {
				readLine(r, l)
			}
		})
	}
	var reqs sync.WaitGroup
	br := bufio.NewReader(in)
	code := 0
	for {
		h, body, err := readFrame(br)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad frame: %v\n", err)
			code = 2
			break
		}
		var req request
		if err := json.Unmarshal(h, &req); err != nil {
			fmt.Fprintf(os.Stderr, "bad header: %v\n", err)
			code = 2
			break
		}
		switch req.Op {
		case "ping":
			send(answer{ID: req.ID, OK: true})
		case "ocr":
			reqs.Go(func() { send(recognize(det, lines, req, body)) })
		default:
			send(answer{ID: req.ID, Error: "internal", Message: "unknown op"})
		}
	}
	reqs.Wait()
	close(lines)
	workers.Wait()
	return code
}

func recognize(det *detector, lines chan<- line, req request, body []byte) answer {
	img, err := png.Decode(bytes.NewReader(body))
	if err != nil {
		return answer{ID: req.ID, Error: "bad_image", Message: "cannot decode PNG"}
	}
	r := req.Region
	region := image.Rect(r[0], r[1], r[2], r[3]).Intersect(img.Bounds())
	boxes, err := det.detect(img, region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "det: %v\n", err)
		return answer{ID: req.ID, Error: "internal", Message: "text detection failed"}
	}
	found := make([]item, len(boxes))
	var done sync.WaitGroup
	done.Add(len(boxes))
	for i, b := range boxes {
		lines <- line{img: img, box: b, out: &found[i], done: &done}
	}
	done.Wait()
	items := []item{}
	for _, it := range found {
		if it.Text != "" {
			items = append(items, it)
		}
	}
	return answer{ID: req.ID, Items: &items}
}

func readLine(r *reader, l line) {
	defer l.done.Done()
	crop := l.box.Inset(-pad).Intersect(l.img.Bounds())
	text, conf := r.read(rgbOf(l.img, crop), crop.Dx(), crop.Dy())
	b := l.box
	*l.out = item{Text: text, Confidence: conf, Bounds: [4]int{b.Min.X, b.Min.Y, b.Max.X, b.Max.Y}}
}
