//go:build embedffmpeg

package video

import _ "embed"

// These files are produced by the CI job "windows-single-exe" (never committed:
// the binary is third party and ~55 MB compressed).
//
//go:embed embedded/ffmpeg.exe.gz
var ffmpegGz []byte

//go:embed embedded/FFMPEG-LICENSE.txt
var ffmpegLicense []byte

func embeddedFFmpeg() (gz, license []byte, ok bool) { return ffmpegGz, ffmpegLicense, true }
