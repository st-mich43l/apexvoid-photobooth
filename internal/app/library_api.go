package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/printing"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
)

// LibrarySummary is shown on the dashboard.
type LibrarySummary struct {
	Sessions    int      `json:"sessions"`
	Photos      int      `json:"photos"`
	Videos      int      `json:"videos"`
	Interrupted []string `json:"interrupted"`
	Recovered   int      `json:"recovered"`
	Missing     int      `json:"missing"`
	FreeBytes   int64    `json:"freeBytes"`
}

func (s *Service) ListSessionPrintJobs(id string, limit, offset int) ([]printing.Job, error) {
	jobs, err := printing.NewSQLStore(s.db).ListSession(id, limit, offset)
	if jobs == nil {
		jobs = []printing.Job{}
	}
	return jobs, userErr(err)
}

func osName() string { return runtime.GOOS }

func (s *Service) librarySummary() LibrarySummary {
	var ls LibrarySummary
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&ls.Sessions)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM media WHERE kind='photo' AND status IN ('active','partial')`).Scan(&ls.Photos)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM media WHERE kind='video' AND status IN ('active','partial')`).Scan(&ls.Videos)
	ls.Interrupted, ls.Recovered, ls.Missing = s.library.Interrupted, len(s.library.Recovered), len(s.library.Missing)
	ls.FreeBytes = freeBytes(s.db.Layout.Root)
	if ls.Interrupted == nil {
		ls.Interrupted = []string{}
	}
	return ls
}

// LibrarySummary returns counts and free disk space.
func (s *Service) LibrarySummary() LibrarySummary { return s.librarySummary() }

// ListSessions lists sessions, newest first.
func (s *Service) ListSessions(limit, offset int) ([]sessions.Session, error) {
	l, err := s.store.List(limit, offset)
	if l == nil {
		l = []sessions.Session{}
	}
	return l, userErr(err)
}

// QuerySessions returns a cursor-paginated, server-filtered session catalogue.
func (s *Service) QuerySessions(q sessions.SessionQuery) (sessions.SessionPage, error) {
	page, err := s.store.QuerySessions(q)
	return page, userErr(err)
}

// ListMedia lists media of a session ("" = whole library), newest first.
func (s *Service) ListMedia(sessionID string, limit, offset int) ([]sessions.Media, error) {
	l, err := s.store.ListMedia(sessionID, limit, offset)
	if l == nil {
		l = []sessions.Media{}
	}
	return l, userErr(err)
}

// QuerySessionMedia lists a bounded page in capture order.
func (s *Service) QuerySessionMedia(id string, limit, offset int) (sessions.MediaPage, error) {
	page, err := s.store.QuerySessionMedia(id, limit, offset)
	return page, userErr(err)
}

// GetMedia returns one media record.
func (s *Service) GetMedia(id string) (sessions.Media, error) {
	m, err := s.store.Media(id)
	return m, userErr(err)
}

// SaveEdits stores a non-destructive edit recipe (rotate/crop/brightness/
// contrast/saturation). The original file is never modified.
func (s *Service) SaveEdits(mediaID string, e media.Edits) (sessions.Media, error) {
	if err := s.guardWrite(); err != nil {
		return sessions.Media{}, err
	}
	m, err := s.store.SaveEdits(mediaID, e)
	return m, userErr(err)
}

// ExportMedia copies the original (or edited render) to destDir ("" = the
// session's exports folder) and returns the new path. Never overwrites.
func (s *Service) ExportMedia(mediaID, destDir string, edited bool) (string, error) {
	p, err := s.store.Export(mediaID, destDir, edited)
	if err != nil {
		return "", userErr(err)
	}
	s.log.Info("exported", "media", mediaID, "to", p)
	return p, nil
}

// TrashMedia moves a media item to the recoverable trash. The UI must have
// asked the operator for confirmation.
func (s *Service) TrashMedia(id string) error {
	if err := s.guardWrite(); err != nil {
		return err
	}
	item, err := s.store.Media(id)
	if err != nil {
		return userErr(err)
	}
	session, err := s.store.Session(item.SessionID)
	if err != nil {
		return userErr(err)
	}
	if session.Status == sessions.StatusActive {
		return newUserErr("This session is still active.", "Finish the session before moving its media to trash.")
	}
	return userErr(s.store.Trash(id))
}

// RestoreMedia brings a trashed item back.
func (s *Service) RestoreMedia(id string) error {
	if err := s.guardWrite(); err != nil {
		return err
	}
	return userErr(s.store.Restore(id))
}

// TrashSession trashes every item of a session.
func (s *Service) TrashSession(id string) error {
	if err := s.guardWrite(); err != nil {
		return err
	}
	return userErr(s.store.TrashSession(id))
}

// RestoreSession resumes a session moved to recoverable trash.
func (s *Service) RestoreSession(id string) error {
	if err := s.guardWrite(); err != nil {
		return err
	}
	return userErr(s.store.RestoreSession(id))
}

// ExportSession streams a local ZIP without overwriting an existing export.
func (s *Service) ExportSession(id, dest string, edited bool) (string, error) {
	if err := s.guardWrite(); err != nil {
		return "", err
	}
	p, err := s.store.ExportSession(id, dest, edited)
	return p, userErr(err)
}

// EmptyTrash permanently deletes trashed items and returns how many.
func (s *Service) EmptyTrash() (int, error) {
	if err := s.guardWrite(); err != nil {
		return 0, err
	}
	n, err := s.store.EmptyTrash()
	return n, userErr(err)
}

// VerifyLibrary runs the integrity check (deep = verify checksums).
func (s *Service) VerifyLibrary(deep bool) (sessions.Report, error) {
	r, err := s.store.Verify(deep)
	if err == nil {
		s.library = r
	}
	return r, userErr(err)
}

// RevealMedia shows the media file in the OS file manager.
func (s *Service) RevealMedia(id string) error {
	p, err := s.store.FilePath(id, "original")
	if err != nil {
		return userErr(err)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", "/select,"+filepath.FromSlash(p))
	case "darwin":
		cmd = exec.Command("open", "-R", p)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(p))
	}
	return userErr(cmd.Start())
}

// PickFolder opens the native folder chooser.
func (s *Service) PickFolder() (string, error) {
	if s.opts.PickFolder == nil {
		return "", newUserErr("Folder selection is not available.", "Type the folder path instead.")
	}
	p, err := s.opts.PickFolder()
	return p, userErr(err)
}

// recoverVideos registers MP4 files that exist on disk without a database
// row (a crash during recording). They are playable thanks to fragmented MP4
// and are flagged "partial".
func (s *Service) recoverVideos() {
	files, _ := filepath.Glob(filepath.Join(s.db.Layout.SessionsDir(), "*", "videos", "VID_*.mp4"))
	for _, f := range files {
		rel, err := s.db.Layout.Rel(f)
		if err != nil {
			continue
		}
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM media WHERE rel_path=?`, rel).Scan(&n)
		if n > 0 {
			continue
		}
		st, err := os.Stat(f)
		if err != nil || st.Size() == 0 {
			continue
		}
		sid := filepath.Base(filepath.Dir(filepath.Dir(f)))
		id := fmt.Sprintf("M%s", trimVid(filepath.Base(f)))
		in := sessions.VideoInput{AbsPath: f, Partial: true, Source: "recovered after interrupted recording"}
		if ff, err := findFFmpeg(s); err == nil {
			if p, err := probeVideo(ff, f); err == nil {
				in.DurationMs, in.Width, in.Height = p.Duration.Milliseconds(), p.Width, p.Height
			}
		}
		if _, err := s.store.AddVideo(sid, id, in); err == nil {
			_ = s.store.SetStatus(sid, sessions.StatusInterrupted, "recording recovered")
			s.library.Recovered = append(s.library.Recovered, id)
			s.log.Warn("recovered video", "file", f)
		}
	}
}

func trimVid(name string) string {
	n := name[len("VID_"):]
	return n[:len(n)-len(filepath.Ext(n))]
}
