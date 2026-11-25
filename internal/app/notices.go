package app

import (
	"fmt"
	"os"

	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
	"github.com/st-mich43l/apexvoid-photobooth/internal/video"
)

// WriteThirdPartyNotices exports licenses carried by the executable. It never
// overwrites an existing file; this also works without opening a library.
func WriteThirdPartyNotices(path string) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if e := f.Close(); err == nil {
			err = e
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	_, err = fmt.Fprintf(f, "ApexVoid Photobooth — third-party notices\n\nNoto Sans Regular: Copyright 2018 The Noto Project Authors.\nSource: https://github.com/notofonts/noto-fonts\n\n%s\n\nFFmpeg: separate process; replaceable by configuring your own FFmpeg.\nSource: https://ffmpeg.org/; build scripts: https://github.com/BtbN/FFmpeg-Builds\n\n%s\n", templates.FontLicense(), video.EmbeddedLicense())
	return err
}
