//go:build !windows

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchArt(t *testing.T) {
	var gotTerm string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".jpg") {
			if !strings.Contains(r.URL.Path, "300x300bb") {
				t.Errorf("art not upsized: %s", r.URL.Path)
			}
			img := image.NewRGBA(image.Rect(0, 0, 300, 300))
			for i := range img.Pix {
				img.Pix[i] = 200
			}
			img.Set(0, 0, color.RGBA{255, 0, 0, 255})
			var b bytes.Buffer
			jpeg.Encode(&b, img, nil)
			w.Write(b.Bytes())
			return
		}
		gotTerm = r.URL.Query().Get("term")
		fmt.Fprintf(w, `{"results":[{"artistName":"Someone Else","trackName":"X","artworkUrl100":"%[1]s/a/100x100bb.jpg","trackTimeMillis":1000},
			{"artistName":"The Weeknd","trackName":"Blinding Lights","artworkUrl100":"%[1]s/b/100x100bb.jpg","trackTimeMillis":200040}]}`, srv.URL)
	}))
	defer srv.Close()
	artBase = srv.URL + "/search"
	img, dur, err := FetchArt("The Weeknd", "Blinding Lights - Remastered 2020")
	if err != nil {
		t.Fatal(err)
	}
	if gotTerm != "The Weeknd Blinding Lights" {
		t.Errorf("term = %q", gotTerm)
	}
	if dur < 200 || dur > 200.1 {
		t.Errorf("dur = %v (should pick the matching artist)", dur)
	}
	if img.W != 300 || img.H != 300 || img.Pix[3] != 1 {
		t.Errorf("bad image %dx%d a=%v", img.W, img.H, img.Pix[3])
	}
}
