package mock_test

import (
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/cameratest"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/mock"
)

func TestContract(t *testing.T) {
	d := mock.NewDriver()
	d.Width, d.Height = 640, 480
	cameratest.RunContract(t, d, cameratest.Options{WantSimulated: true})
}
