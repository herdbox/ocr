package main

import (
	"fmt"
	"image"
	"math"
	"path/filepath"
	"runtime"

	ort "github.com/yalue/onnxruntime_go"
)

// detLong is the long side the det model sees.
const detLong = 960

// ortLibrary is the ONNX Runtime library inside the package folder.
func ortLibrary() string {
	if runtime.GOOS == "windows" {
		return "onnxruntime.dll"
	}
	return "libonnxruntime.so.1.31.1"
}

// detector finds text lines with PP-OCRv5 mobile det. One session serves all
// workers: ONNX Runtime's Run is safe for concurrent use.
type detector struct {
	sess    *ort.DynamicAdvancedSession
	in, out string
}

func newDetector(dir, lib string) (*detector, error) {
	ort.SetSharedLibraryPath(lib)
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("onnxruntime: %w", err)
	}
	model := filepath.Join(dir, "det.onnx")
	ins, outs, err := ort.GetInputOutputInfo(model)
	if err != nil {
		return nil, fmt.Errorf("det model: %w", err)
	}
	if len(ins) != 1 || len(outs) != 1 {
		return nil, fmt.Errorf("det model: %d inputs, %d outputs", len(ins), len(outs))
	}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	if err := opts.SetIntraOpNumThreads(1); err != nil {
		return nil, err
	}
	if err := opts.SetInterOpNumThreads(1); err != nil {
		return nil, err
	}
	sess, err := ort.NewDynamicAdvancedSession(model, []string{ins[0].Name}, []string{outs[0].Name}, opts)
	if err != nil {
		return nil, fmt.Errorf("det session: %w", err)
	}
	return &detector{sess: sess, in: ins[0].Name, out: outs[0].Name}, nil
}

// detect returns line boxes in img's pixels, inside region.
func (d *detector) detect(img image.Image, region image.Rectangle) ([]image.Rectangle, error) {
	rw, rh := region.Dx(), region.Dy()
	if rw <= 0 || rh <= 0 {
		return nil, nil
	}
	w, h := detSize(rw, rh)
	input := detInput(img, region, w, h)
	in, err := ort.NewTensor(ort.NewShape(1, 3, int64(h), int64(w)), input)
	if err != nil {
		return nil, err
	}
	defer in.Destroy()
	out, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 1, int64(h), int64(w)))
	if err != nil {
		return nil, err
	}
	defer out.Destroy()
	if err := d.sess.Run([]ort.Value{in}, []ort.Value{out}); err != nil {
		return nil, err
	}
	boxes := detBoxes(out.GetData(), w, h)
	sx, sy := float64(rw)/float64(w), float64(rh)/float64(h)
	var res []image.Rectangle
	for _, b := range boxes {
		r := image.Rect(
			region.Min.X+int(math.Floor(float64(b.Min.X)*sx)),
			region.Min.Y+int(math.Floor(float64(b.Min.Y)*sy)),
			region.Min.X+int(math.Ceil(float64(b.Max.X)*sx)),
			region.Min.Y+int(math.Ceil(float64(b.Max.Y)*sy)),
		).Intersect(region)
		if !r.Empty() {
			res = append(res, r)
		}
	}
	return res, nil
}

// detSize scales the long side to detLong and rounds each side to a multiple
// of 32, at least 32.
func detSize(w, h int) (int, int) {
	ratio := float64(detLong) / float64(max(w, h))
	round := func(v int) int {
		return max(32, int(math.Round(float64(v)*ratio/32))*32)
	}
	return round(w), round(h)
}

// Normalization of the det model, applied in BGR channel order as PaddleOCR
// does.
var (
	detMean = [3]float32{0.485, 0.456, 0.406}
	detStd  = [3]float32{0.229, 0.224, 0.225}
)

// detInput resizes region of img to w×h (bilinear) and returns it as NCHW
// float32, BGR, normalized.
func detInput(img image.Image, region image.Rectangle, w, h int) []float32 {
	src := rgbOf(img, region)
	sw, sh := region.Dx(), region.Dy()
	plane := w * h
	out := make([]float32, 3*plane)
	sx, sy := float64(sw)/float64(w), float64(sh)/float64(h)
	for y := range h {
		fy := (float64(y)+0.5)*sy - 0.5
		y0 := int(math.Floor(fy))
		wy := float32(fy - float64(y0))
		y0c, y1c := clampInt(y0, 0, sh-1), clampInt(y0+1, 0, sh-1)
		for x := range w {
			fx := (float64(x)+0.5)*sx - 0.5
			x0 := int(math.Floor(fx))
			wx := float32(fx - float64(x0))
			x0c, x1c := clampInt(x0, 0, sw-1), clampInt(x0+1, 0, sw-1)
			for c := range 3 {
				// src is RGB; channel c of the output is BGR.
				sc := 2 - c
				p00 := float32(src[(y0c*sw+x0c)*3+sc])
				p01 := float32(src[(y0c*sw+x1c)*3+sc])
				p10 := float32(src[(y1c*sw+x0c)*3+sc])
				p11 := float32(src[(y1c*sw+x1c)*3+sc])
				v := (p00*(1-wx)+p01*wx)*(1-wy) + (p10*(1-wx)+p11*wx)*wy
				out[c*plane+y*w+x] = (v/255 - detMean[c]) / detStd[c]
			}
		}
	}
	return out
}

// rgbOf returns region of img as packed RGB bytes.
func rgbOf(img image.Image, region image.Rectangle) []byte {
	w, h := region.Dx(), region.Dy()
	out := make([]byte, w*h*3)
	switch m := img.(type) {
	case *image.NRGBA:
		for y := range h {
			row := m.Pix[m.PixOffset(region.Min.X, region.Min.Y+y):]
			for x := range w {
				copy(out[(y*w+x)*3:], row[x*4:x*4+3])
			}
		}
	case *image.RGBA:
		for y := range h {
			row := m.Pix[m.PixOffset(region.Min.X, region.Min.Y+y):]
			for x := range w {
				copy(out[(y*w+x)*3:], row[x*4:x*4+3])
			}
		}
	default:
		for y := range h {
			for x := range w {
				r, g, b, _ := img.At(region.Min.X+x, region.Min.Y+y).RGBA()
				i := (y*w + x) * 3
				out[i], out[i+1], out[i+2] = byte(r>>8), byte(g>>8), byte(b>>8)
			}
		}
	}
	return out
}

func clampInt(v, lo, hi int) int { return min(max(v, lo), hi) }
