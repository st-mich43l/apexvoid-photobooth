package templates

import (
	"image"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
)

func encode(img *image.NRGBA, dpi int) ([]byte, int, int, error) {
	enc, err := media.EncodeJPEG(img, 95)
	if err != nil {
		return nil, 0, 0, err
	}
	b := img.Bounds()
	return media.SetJPEGDensity(enc, dpi), b.Dx(), b.Dy(), nil
}
