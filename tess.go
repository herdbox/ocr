package main

/*
#include <stdlib.h>
#include <tesseract/capi.h>
*/
import "C"

import (
	"errors"
	"math"
	"strings"
	"unsafe"
)

// reader reads one text line with Tesseract vie. A TessBaseAPI is not safe
// for concurrent use: each worker owns one reader.
type reader struct {
	api *C.TessBaseAPI
}

func newReader(tessdata string) (*reader, error) {
	api := C.TessBaseAPICreate()
	path := C.CString(tessdata)
	defer C.free(unsafe.Pointer(path))
	lang := C.CString("vie")
	defer C.free(unsafe.Pointer(lang))
	if C.TessBaseAPIInit3(api, path, lang) != 0 {
		C.TessBaseAPIDelete(api)
		return nil, errors.New("tesseract: cannot load vie data")
	}
	C.TessBaseAPISetPageSegMode(api, C.PSM_SINGLE_LINE)
	return &reader{api: api}, nil
}

// read returns the text of a packed RGB image and its confidence (0–1).
func (r *reader) read(rgb []byte, w, h int) (string, float64) {
	if w <= 0 || h <= 0 {
		return "", 0
	}
	C.TessBaseAPISetImage(r.api, (*C.uchar)(unsafe.Pointer(&rgb[0])), C.int(w), C.int(h), 3, C.int(w*3))
	// Line crops carry no resolution; set Tesseract's own fallback so it does
	// not warn on stderr.
	C.TessBaseAPISetSourceResolution(r.api, 70)
	txt := C.TessBaseAPIGetUTF8Text(r.api)
	if txt == nil {
		C.TessBaseAPIClear(r.api)
		return "", 0
	}
	text := strings.TrimSpace(C.GoString(txt))
	C.TessDeleteText(txt)
	conf := float64(C.TessBaseAPIMeanTextConf(r.api))
	C.TessBaseAPIClear(r.api)
	return text, math.Round(max(0, min(conf, 100))*10) / 1000
}

func (r *reader) close() { C.TessBaseAPIDelete(r.api) }
