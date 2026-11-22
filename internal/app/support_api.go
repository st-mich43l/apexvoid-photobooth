package app

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// testNote marks a session as an operator's camera test, so it is never mistaken for a guest session.
const testNote = "Operator camera test"

// LabelTestSession marks a finished (or open) session as an operator camera test. It only adds the note:
// nothing is deleted, moved or hidden, and the originals stay in the library like any other session.
func (s *Service) LabelTestSession(sessionID string) error {
	if err := s.guardWrite(); err != nil {
		return err
	}
	ss, err := s.store.Session(sessionID)
	if err != nil {
		return newUserErr("That session was not found.", "")
	}
	if err := s.store.SetStatus(sessionID, ss.Status, testNote); err != nil {
		return newUserErr(err.Error(), "")
	}
	return nil
}

var (
	winUser  = regexp.MustCompile(`(?i)[a-z]:\\users\\[^\\/\s"'<>|:*?]+`)
	unixUser = regexp.MustCompile(`/(?:home|Users)/[^/\s"']+`)
)

// redactor removes what must not leave the machine in a support summary: the user's folders, the media
// and settings locations, the printer's name and anything that looks like a PIN setting.
func (s *Service) redactor() func(string) string {
	cfg := s.settings.Get()
	pairs := [][2]string{}
	add := func(v, with string) {
		if len(v) >= 3 {
			pairs = append(pairs, [2]string{v, with})
		}
	}
	add(s.db.Layout.Root, "<media folder>")
	add(cfg.MediaDir, "<media folder>")
	add(cfg.FFmpegPath, "<ffmpeg>")
	add(cfg.CanonSDKDir, "<canon sdk folder>")
	add(cfg.PrinterName, "<printer>")
	if h, err := os.UserHomeDir(); err == nil {
		add(h, "<home>")
	}
	if h, err := os.Hostname(); err == nil {
		add(h, "<computer>")
	}
	return func(in string) string {
		for _, p := range pairs {
			in = strings.ReplaceAll(in, p[0], p[1])
		}
		in = winUser.ReplaceAllString(in, `<user profile>`)
		return unixUser.ReplaceAllString(in, "<user profile>")
	}
}

// DiagnosticSummary is a plain-text report an operator can paste into a support request. It contains status
// and counts only: no PIN, no secrets, no photos or file names, no printer name, no user or computer names
// and no folder paths.
func (s *Service) DiagnosticSummary() string {
	red := s.redactor()
	r := s.BoothReadiness()
	d := s.RunDiagnostics()
	cs := s.mgr.Status()
	cfg := s.settings.Get()
	var b strings.Builder
	line := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	line("ApexVoid Photobooth — support summary")
	line("Generated: %s", time.Now().Format(time.RFC3339))
	line("Version: %s   Platform: %s/%s", s.opts.Version, runtime.GOOS, runtime.GOARCH)
	line("")
	line("Booth: %s (guests can start: %v)", r.Headline, r.CanStart)
	if r.Demo {
		line("NOTE: a SIMULATED camera is in use — this is demo/test mode, not hardware verification.")
	}
	line("Printing offered to guests: %s", r.Printing)
	if cs.Device != nil {
		line("Photo camera: %s — backend %s, kind %s, state %s, reconnecting %v, simulated %v", cs.Device.Name, cs.Device.Driver, cs.Device.Kind, cs.State, cs.Reconnecting, cs.Device.Simulated)
	} else {
		line("Photo camera: none connected")
	}
	line("Printer configured: %v   Countdown: %ds   Mirror preview: %v   DPI: %d", cfg.PrinterName != "", cfg.CountdownSeconds, cfg.PreviewMirror, cfg.OutputDPI)
	line("PIN protection: %v (the PIN itself is never exported)", cfg.KioskPin != "")
	line("")
	line("Readiness:")
	for _, it := range r.Items {
		req := "optional"
		if it.Required {
			req = "required"
		}
		v := ""
		if it.Verification != "" {
			v = " [" + it.Verification + "]"
		}
		line("  - %s (%s, %s)%s: %s", it.Label, req, it.State, v, red(it.Detail))
	}
	line("")
	line("Diagnostics:")
	for _, c := range d.Checks {
		line("  - [%s%s] %s: %s", c.State, map[bool]string{true: ", blocks guests", false: ""}[c.BlocksGuests], c.Name, red(c.Detail))
	}
	line("")
	line("Library: %d photos, %d videos, %d sessions, %d interrupted, free %s", d.Library.Photos, d.Library.Videos, d.Library.Sessions, len(d.Library.Interrupted), humanBytes(d.Library.FreeBytes))
	line("Excluded on purpose: PIN, file names and photo content, folder paths, printer name, user and computer names.")
	return b.String()
}
