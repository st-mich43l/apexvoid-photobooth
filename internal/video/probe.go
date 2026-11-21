package video

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reDuration = regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+(?:\.\d+)?)`)
	reStream   = regexp.MustCompile(`Stream #\d+:\d+[^:]*:\s*(Video|Audio):\s*([A-Za-z0-9_]+)(.*)`)
	reSize     = regexp.MustCompile(`[ ,](\d{2,5})x(\d{2,5})[ ,\[]`)
)

func parseProbe(out string) Probe {
	var p Probe
	if m := reDuration.FindStringSubmatch(out); m != nil {
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		s, _ := strconv.ParseFloat(m[3], 64)
		p.Duration = time.Duration((float64(h*3600+mi*60) + s) * float64(time.Second))
	}
	for _, line := range strings.Split(out, "\n") {
		m := reStream.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch m[1] {
		case "Video":
			if p.HasVideo {
				continue
			}
			p.HasVideo, p.VCodec = true, m[2]
			if sz := reSize.FindStringSubmatch(m[3] + " "); sz != nil {
				p.Width, _ = strconv.Atoi(sz[1])
				p.Height, _ = strconv.Atoi(sz[2])
			}
		case "Audio":
			p.HasAudio, p.ACodec = true, m[2]
		}
	}
	return p
}
