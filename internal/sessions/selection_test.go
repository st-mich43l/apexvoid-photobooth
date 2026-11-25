package sessions

import (
	"image/color"
	"os"
	"sort"
	"testing"
)

// session with n candidate photographs (what the engine stores while the guest is still choosing)
func candidates(t *testing.T, s *Store, n int) (Session, []Media) {
	t.Helper()
	sess, err := s.Create("photo", n, "t", false)
	if err != nil {
		t.Fatal(err)
	}
	var ms []Media
	for i := 0; i < n; i++ {
		m, err := s.AddPhoto(sess.ID, PhotoInput{Slot: i, Data: jpegBytes(t, 60, 40, color.NRGBA{R: uint8(30 * i), G: 90, B: 160}), Candidate: true})
		if err != nil {
			t.Fatal(err)
		}
		ms = append(ms, m)
	}
	return sess, ms
}

func exists(t *testing.T, db interface{}, l func(string) (string, error), m Media) bool {
	t.Helper()
	p, err := l(m.RelPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(p)
	return err == nil
}

func statuses(t *testing.T, s *Store, session string) map[string]string {
	t.Helper()
	rows, err := s.db.Query(`SELECT id,status FROM media WHERE session_id=? AND kind='photo'`, session)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, st string
		_ = rows.Scan(&id, &st)
		out[id] = st
	}
	return out
}

func TestSelectPhotosIsOneAtomicStepThatDeletesNothing(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, ms := candidates(t, s, 8)
	if err := s.SelectPhotos(sess.ID, []int{5, 1, 7}); err != nil {
		t.Fatal(err)
	}
	st := statuses(t, s, sess.ID)
	for i, m := range ms {
		want := MediaDiscarded
		if i == 5 || i == 1 || i == 7 {
			want = MediaActive
		}
		if st[m.ID] != want {
			t.Fatalf("photo %d is %q, want %q", i, st[m.ID], want)
		}
		if !exists(t, db, db.Layout.Abs, m) {
			t.Fatalf("photo %d: the original must still be on disk", i)
		}
	}
	got, _ := s.ListMedia(sess.ID, 50, 0)
	sort.Slice(got, func(i, j int) bool { return got[i].Slot < got[j].Slot })
	if len(got) != 3 || got[0].ID != ms[5].ID || got[1].ID != ms[1].ID || got[2].ID != ms[7].ID || got[0].Slot != 0 || got[2].Slot != 2 {
		t.Fatalf("selection order must become slot order: %+v", got)
	}
}

func TestSelectPhotosRefusesBadSelectionsAndChangesNothing(t *testing.T) {
	s, _ := open(t, t.TempDir())
	sess, ms := candidates(t, s, 4)
	for name, sel := range map[string][]int{"duplicate": {1, 1}, "unrelated": {1, 9}, "negative": {-2}} {
		if err := s.SelectPhotos(sess.ID, sel); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	st := statuses(t, s, sess.ID)
	for _, m := range ms {
		if st[m.ID] != MediaCandidate {
			t.Fatalf("a refused selection must leave every candidate untouched: %v", st)
		}
	}
	other, _ := candidates(t, s, 2)
	if err := s.SelectPhotos(sess.ID, []int{0, 1}); err != nil {
		t.Fatal(err)
	}
	if st2 := statuses(t, s, other.ID); len(st2) != 2 {
		t.Fatal("another session must be unaffected")
	}
}

func TestPruneAfterSelectionRemovesOnlyTheUnselectedAndCanBeRepeated(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, ms := candidates(t, s, 6)
	_ = s.SelectPhotos(sess.ID, []int{2, 4})
	if err := s.PruneDiscardedPhotos(sess.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneDiscardedPhotos(sess.ID); err != nil { // repeating is harmless
		t.Fatal(err)
	}
	st := statuses(t, s, sess.ID)
	if len(st) != 2 || st[ms[2].ID] != MediaActive || st[ms[4].ID] != MediaActive {
		t.Fatalf("%v", st)
	}
	for _, i := range []int{2, 4} {
		if !exists(t, db, db.Layout.Abs, ms[i]) {
			t.Fatalf("selected original %d lost", i)
		}
	}
	for _, i := range []int{0, 1, 3, 5} {
		if exists(t, db, db.Layout.Abs, ms[i]) {
			t.Fatalf("unselected original %d should be deleted", i)
		}
	}
}

func TestRestoreGivesEveryPhotoBackWhenTheStripFailed(t *testing.T) {
	s, _ := open(t, t.TempDir())
	sess, ms := candidates(t, s, 5)
	_ = s.SelectPhotos(sess.ID, []int{3, 0})
	if err := s.RestoreDiscardedPhotos(sess.ID); err != nil {
		t.Fatal(err)
	}
	st := statuses(t, s, sess.ID)
	for _, m := range ms {
		if st[m.ID] != MediaActive {
			t.Fatalf("%v", st)
		}
	}
	got, _ := s.ListMedia(sess.ID, 50, 0)
	sort.Slice(got, func(i, j int) bool { return got[i].Slot < got[j].Slot })
	if got[0].ID != ms[3].ID || got[1].ID != ms[0].ID {
		t.Fatalf("the chosen photos keep their order at the front: %+v", got)
	}
}

// A crash between choosing and deleting must not lose anything: start-up recovery gives the photos back.
func TestCrashAfterSelectionLosesNoPhoto(t *testing.T) {
	root := t.TempDir()
	s, db := open(t, root)
	sess, ms := candidates(t, s, 8)
	if err := s.SelectPhotos(sess.ID, []int{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	// ... the process dies here: no prune, no finalize, the session is still "active"
	db.Close()
	s2, _ := open(t, root)
	rep, err := s2.Verify(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Interrupted) != 1 {
		t.Fatalf("%+v", rep)
	}
	st := statuses(t, s2, sess.ID)
	for _, m := range ms {
		if st[m.ID] != MediaActive {
			t.Fatalf("photo %s is %q after recovery: nothing may be lost", m.ID, st[m.ID])
		}
	}
}
