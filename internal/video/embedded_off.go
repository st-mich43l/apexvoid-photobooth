//go:build !embedffmpeg

package video

// embeddedFFmpeg: the normal build embeds nothing.
func embeddedFFmpeg() (gz, license []byte, ok bool) { return nil, nil, false }
