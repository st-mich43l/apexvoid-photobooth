//go:build uxharness

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/app"
	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

// galleryScenario is one synthetic session for Gallery session-detail screenshots. Photos are generated
// gradients (never real guests); compositions are rendered afterwards by the REAL frame renderer.
type galleryScenario struct {
	note     string
	mode     string
	photos   int
	w, h     int
	videos   []sessions.VideoInput
	compose  []string // layout suffixes, e.g. "sheet4"; the first frame offering that layout is used
	design   string   // preferred design prefix
	prints   int      // print jobs submitted for the newest composition
	editSlot int      // -1 = none
	status   string
}

var galleryScenarios = []galleryScenario{
	{note: "4-photo 4x6", mode: "photo", photos: 4, w: 1600, h: 1067, compose: []string{"sheet4"}, design: "lavender", prints: 2, editSlot: 1, status: sessions.StatusCompleted},
	{note: "3-photo 2x6", mode: "photo", photos: 3, w: 1600, h: 1067, compose: []string{"strip3"}, design: "rose-pink", editSlot: -1, status: sessions.StatusCompleted},
	{note: "8-photo", mode: "photo", photos: 8, w: 1600, h: 1067, compose: []string{"sheet6"}, design: "film-strip", editSlot: -1, status: sessions.StatusCompleted},
	{note: "video-only", mode: "video", w: 1920, h: 1080, videos: []sessions.VideoInput{{DurationMs: 24_500, Width: 1920, Height: 1080}, {DurationMs: 8_200, Width: 1920, Height: 1080, Partial: true}}, editSlot: -1, status: sessions.StatusCompleted},
	{note: "multiple compositions", mode: "photo", photos: 4, w: 1067, h: 1600, compose: []string{"strip4", "sheet4", "twoup4"}, design: "cream-beige", editSlot: -1, status: sessions.StatusCompleted},
	{note: "no composition", mode: "photo", photos: 2, w: 1600, h: 1067, editSlot: -1, status: sessions.StatusInterrupted},
}

type seededSession struct {
	id string
	sc galleryScenario
}

// seedGalleryStore writes the scenarios' photos and videos straight into the library (before the service starts).
func seedGalleryStore(root string) []seededSession {
	db, err := storage.Open(storage.Layout{Root: root})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	store := sessions.New(db)
	var out []seededSession
	base := time.Now().UTC().Truncate(time.Minute)
	for i, sc := range galleryScenarios {
		t := base.Add(-time.Duration(len(galleryScenarios)-i) * 47 * time.Minute)
		store.SetClock(func() time.Time { return t })
		shots := sc.photos
		if shots == 0 {
			shots = 1
		}
		sess, err := store.Create(sc.mode, shots, "", false)
		if err != nil {
			log.Fatal(err)
		}
		for p := 0; p < sc.photos; p++ {
			if _, err := store.AddPhoto(sess.ID, sessions.PhotoInput{Slot: p, Data: samplePhoto(sc.w, sc.h, i*7+p), Source: "generated screenshot sample"}); err != nil {
				log.Fatal(err)
			}
		}
		for v, in := range sc.videos {
			id, abs, err := store.NewVideoPath(sess.ID)
			if err != nil {
				log.Fatal(err)
			}
			if err := os.WriteFile(abs, []byte("not a real video: screenshot harness"), 0o644); err != nil {
				log.Fatal(err)
			}
			in.AbsPath, in.Source = abs, "generated screenshot sample"
			in.ThumbJPEG = samplePhoto(480, 270, 40+v)
			if _, err := store.AddVideo(sess.ID, id, in); err != nil {
				log.Fatal(err)
			}
		}
		if err := store.SetStatus(sess.ID, sc.status, ""); err != nil {
			log.Fatal(err)
		}
		out = append(out, seededSession{id: sess.ID, sc: sc})
	}
	return out
}

// finishGallerySeed renders compositions, applies an edit and submits print jobs through the real service.
func finishGallerySeed(svc *app.Service, seeded []seededSession) {
	frames := svc.ListFrames(true).Frames
	pick := func(layout, design string) (string, string) {
		var fallback string
		for _, f := range frames {
			if !strings.HasSuffix(f.ID, "."+layout) {
				continue
			}
			if fallback == "" {
				fallback = f.ID
			}
			if strings.HasPrefix(f.ID, design+".") {
				v := ""
				if len(f.Variants) > 0 {
					v = f.Variants[0].ID
				}
				return f.ID, v
			}
		}
		return fallback, ""
	}
	for _, s := range seeded {
		var last string
		for _, layout := range s.sc.compose {
			id, variant := pick(layout, s.sc.design)
			m, err := svc.ComposeSession(s.id, id, variant, "", true)
			if err != nil {
				log.Printf("seed %s: compose %s: %v", s.sc.note, layout, err)
				continue
			}
			last = m.ID
		}
		if s.sc.editSlot >= 0 {
			if ms, err := svc.ListMedia(s.id, 50, 0); err == nil {
				for _, m := range ms {
					if m.Kind == "photo" && m.Slot == s.sc.editSlot {
						if _, err := svc.SaveEdits(m.ID, media.Edits{Brightness: 0.12, Contrast: 0.08, Saturation: 0.1}); err != nil {
							log.Printf("seed %s: edit: %v", s.sc.note, err)
						}
					}
				}
			}
		}
		for p := 0; p < s.sc.prints && last != ""; p++ {
			if _, err := svc.PrintMedia(app.PrintOptions{RequestID: fmt.Sprintf("seed-%s-%d", s.id, p), MediaID: last, Printer: "Lab", Paper: "4x6 in", Orientation: "auto", Fit: "fit", Copies: 1 + p}); err != nil {
				log.Printf("seed %s: print: %v", s.sc.note, err)
			}
		}
		log.Printf("seeded gallery session %s (%s)", s.id, s.sc.note)
	}
}

// samplePhoto is a soft, photo-like gradient with a few out-of-focus highlights; obviously synthetic.
func samplePhoto(w, h, seed int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	hue := float64(seed%12) / 12
	a, b := hsl(hue, .45, .32), hsl(math.Mod(hue+.18, 1), .55, .62)
	type spot struct{ x, y, r float64 }
	spots := []spot{{.3, .35, .22}, {.68, .58, .3}, {.5 + .1*math.Sin(float64(seed)), .78, .16}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/float64(w), float64(y)/float64(h)
			t := (fx*.4 + fy*.6)
			r, g, bl := lerp(a[0], b[0], t), lerp(a[1], b[1], t), lerp(a[2], b[2], t)
			for _, s := range spots {
				dx, dy := (fx-s.x)*float64(w)/float64(h), fy-s.y
				if d := math.Sqrt(dx*dx+dy*dy) / s.r; d < 1 {
					k := (1 - d*d) * .35
					r, g, bl = r+(255-r)*k, g+(240-g)*k, bl+(235-bl)*k
				}
			}
			img.Set(x, y, color.RGBA{uint8(r), uint8(g), uint8(bl), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func hsl(h, s, l float64) [3]float64 {
	f := func(n float64) float64 {
		k := math.Mod(n+h*12, 12)
		return 255 * (l - s*math.Min(l, 1-l)*math.Max(-1, math.Min(k-3, math.Min(9-k, 1))))
	}
	return [3]float64{f(0), f(8), f(4)}
}
