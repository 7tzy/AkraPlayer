package main

// Album art: Spotify's window only tells us "Artist - Song", so we look the
// song up in Apple's public iTunes Search API (no account needed), which also
// gives the song length for the progress bar.

import (
	"encoding/json"
	"errors"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var artBase = "https://itunes.apple.com/search"

var artClient = &http.Client{Timeout: 8 * time.Second}

type itunesResult struct {
	Results []struct {
		ArtistName      string `json:"artistName"`
		TrackName       string `json:"trackName"`
		ArtworkURL100   string `json:"artworkUrl100"`
		TrackTimeMillis int    `json:"trackTimeMillis"`
	} `json:"results"`
}

// cleanTitle drops "- Remastered 2011", "(feat. X)" etc. so searches match.
func cleanTitle(t string) string {
	for _, sep := range []string{" - ", " (feat", " (with", " [", " (Remaster", " (Live"} {
		if i := strings.Index(t, sep); i > 0 {
			t = t[:i]
		}
	}
	return strings.TrimSpace(t)
}

// FetchArt returns the cover (as an opaque canvas) and the song length in seconds.
func FetchArt(artist, title string) (*Canvas, float32, error) {
	q := url.Values{}
	q.Set("term", strings.TrimSpace(artist+" "+cleanTitle(title)))
	q.Set("media", "music")
	q.Set("entity", "song")
	q.Set("limit", "5")
	req, _ := http.NewRequest("GET", artBase+"?"+q.Encode(), nil)
	req.Header.Set("User-Agent", "AkraPlayer/2.2")
	resp, err := artClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var res itunesResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, 0, err
	}
	if len(res.Results) == 0 {
		return nil, 0, errors.New("not found")
	}
	// prefer an exact-ish artist match
	best := res.Results[0]
	la := strings.ToLower(artist)
	for _, r := range res.Results {
		if strings.Contains(la, strings.ToLower(r.ArtistName)) || strings.Contains(strings.ToLower(r.ArtistName), la) {
			best = r
			break
		}
	}
	dur := float32(best.TrackTimeMillis) / 1000
	if best.ArtworkURL100 == "" {
		return nil, dur, errors.New("no artwork")
	}
	imgURL := strings.Replace(best.ArtworkURL100, "100x100bb", "300x300bb", 1)
	ir, err := artClient.Get(imgURL)
	if err != nil {
		return nil, dur, err
	}
	defer ir.Body.Close()
	img, _, err := image.Decode(ir.Body)
	if err != nil {
		return nil, dur, err
	}
	return imageToCanvas(img), dur, nil
}

func imageToCanvas(img image.Image) *Canvas {
	b := img.Bounds()
	c := NewCanvas(b.Dx(), b.Dy(), 1)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			px := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			r, g, bl, a := px.R, px.G, px.B, px.A
			af := float32(a) / 255
			i := (y*c.W + x) * 4
			c.Pix[i] = float32(r) / 255 * af
			c.Pix[i+1] = float32(g) / 255 * af
			c.Pix[i+2] = float32(bl) / 255 * af
			c.Pix[i+3] = af
		}
	}
	return c
}
