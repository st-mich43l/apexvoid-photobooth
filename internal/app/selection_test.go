package app

import (
	"sort"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
)

// Eight candidate photographs, a three-photo frame: the guest's choice (in the guest's order) must be what the
// finished strip is made from, and nothing else may remain in the library.
func TestSelectedCandidatesFillTheFrameInSelectionOrder(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	if err := s.StartPreview(); err != nil {
		t.Fatal(err)
	}
	if err := s.StartShootingWithCount(frameFor(3), "", "", 8); err != nil {
		t.Fatal(err)
	}
	rv := waitShoot(t, s, capture.Review)
	if len(rv.Taken) != 8 {
		t.Fatalf("expected eight candidates, got %d", len(rv.Taken))
	}
	by := map[int]string{}
	for _, sh := range rv.Taken {
		by[sh.Slot] = sh.MediaID
	}
	for name, bad := range map[string][]int{"too few": {1, 2}, "duplicate": {1, 1, 2}, "unrelated": {1, 2, 40}} {
		if err := s.AcceptSelectedPhotos(bad); err == nil {
			t.Fatalf("%s selection must be refused by the backend", name)
		}
	}
	if err := s.AcceptSelectedPhotos([]int{6, 2, 4}); err != nil {
		t.Fatal(err)
	}
	done := waitShoot(t, s, capture.Completed)
	if done.ResultID == "" || done.Warning != "" {
		t.Fatalf("the strip must be composed: %+v", done)
	}
	ms, err := s.store.ListMedia(done.SessionID, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var photos []string
	slots := map[string]int{}
	strips := 0
	for _, m := range ms {
		switch m.Kind {
		case "photo":
			photos = append(photos, m.ID)
			slots[m.ID] = m.Slot
		case "strip":
			strips++
		}
	}
	sort.Slice(photos, func(i, j int) bool { return slots[photos[i]] < slots[photos[j]] })
	if len(photos) != 3 || photos[0] != by[6] || photos[1] != by[2] || photos[2] != by[4] || strips != 1 {
		t.Fatalf("photos %v (want %v %v %v), strips %d", photos, by[6], by[2], by[4], strips)
	}
	// a repeated Continue changes nothing
	if err := s.AcceptSelectedPhotos([]int{6, 2, 4}); err == nil {
		t.Fatal("a second accept must be refused")
	}
	if again, _ := s.store.ListMedia(done.SessionID, 100, 0); len(again) != len(ms) {
		t.Fatal("a refused second accept must not compose again")
	}
}

// Changing the frame while the guest is still choosing: the number of photos to choose follows the new frame, and
// the captured candidates are untouched.
func TestFrameChangeDuringReviewChangesTheRequiredSelection(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	_ = s.StartPreview()
	if err := s.StartShootingWithCount(frameFor(3), "", "", 8); err != nil {
		t.Fatal(err)
	}
	rv := waitShoot(t, s, capture.Review)
	if err := s.SetSessionFrame(rv.SessionID, frameFor(4), "", "", false); err != nil {
		t.Fatalf("a frame that the eight candidates can fill must be accepted without 'use the first N': %v", err)
	}
	if err := s.AcceptSelectedPhotos([]int{0, 1, 2}); err == nil {
		t.Fatal("three photos are no longer enough for a four-photo frame")
	}
	if s.ShootState().State != capture.Review || len(s.ShootState().Taken) != 8 {
		t.Fatalf("a refused selection must leave the candidates alone: %+v", s.ShootState())
	}
	if err := s.AcceptSelectedPhotos([]int{7, 0, 3, 5}); err != nil {
		t.Fatal(err)
	}
	if done := waitShoot(t, s, capture.Completed); done.ResultID == "" || len(done.Taken) != 4 {
		t.Fatalf("%+v", done)
	}
}

func TestFrameNeedingMoreThanTheCandidatesIsRefusedInReview(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	_ = s.StartPreview()
	if err := s.StartShootingWithCount(frameFor(2), "", "", 4); err != nil {
		t.Fatal(err)
	}
	rv := waitShoot(t, s, capture.Review)
	if err := s.SetSessionFrame(rv.SessionID, frameFor(6), "", "", false); err == nil {
		t.Fatal("a six-photo frame cannot be filled from four captures")
	}
	if err := s.AcceptSelectedPhotos([]int{0, 1}); err != nil {
		t.Fatalf("the original frame still works: %v", err)
	}
}
