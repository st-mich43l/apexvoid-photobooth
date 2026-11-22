package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
)

func TestCanonStatusNeverClaimsWhatWasNotObserved(t *testing.T) {
	s, err := New(Options{ConfigPath: filepath.Join(t.TempDir(), "settings.json"), SkipRestore: true, DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	st := s.CanonStatus()
	if len(st.Guide) < 5 || !strings.Contains(strings.Join(st.Guide, " "), "Photo Import / Remote Control") {
		t.Fatalf("setup guide missing: %v", st.Guide)
	}
	if !strings.Contains(st.Verification, "neither proves") {
		t.Fatal("the status must say that fakes and CI do not prove real hardware works")
	}
	by := map[string]Readiness{}
	for _, it := range st.Items {
		by[it.Key] = it
	}
	// no Canon SDK in the test environment: that is a problem with an action, never "ok"
	if sdk := by["sdk"]; sdk.State != ReadyProblem || sdk.Action == "" || sdk.Detail == "" {
		t.Fatalf("sdk: %+v", sdk)
	}
	// nothing connected: capabilities are "not tried", never "ok"
	for _, k := range []string{"connected", "liveview", "still", "original"} {
		if by[k].State != ReadyNotTried {
			t.Fatalf("%s must be not_tried without a connected Canon camera: %+v", k, by[k])
		}
	}
	if st.Active != nil || st.Recommendation == "" {
		t.Fatalf("active=%v recommendation=%q", st.Active, st.Recommendation)
	}
}

func TestClassifyCanonErrorDistinguishesTheCauses(t *testing.T) {
	seen := map[string]string{}
	for _, c := range []string{camera.CodeSDKMissing, camera.CodeDeviceBusy, camera.CodeUnsupported, camera.CodeDisconnected, camera.CodeTimeout, camera.CodeCaptureFailed} {
		v := classifyCanonError(c)
		if prev, dup := seen[v]; dup {
			t.Fatalf("%s and %s are described identically (%q)", prev, c, v)
		}
		seen[v] = c
	}
	if classifyCanonError(camera.CodeNotConnected) != classifyCanonError(camera.CodeDisconnected) {
		t.Fatal("not connected and disconnected are the same situation for the operator")
	}
}
