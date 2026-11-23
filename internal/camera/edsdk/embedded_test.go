package edsdk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnpackSDKRepairsAndSeparatesVersions(t *testing.T) {
	base := t.TempDir()
	dir, err := unpackSDK(base, []byte("sdk-one"), []byte("image-one"))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"EDSDK.dll": "sdk-one", "EdsImage.dll": "image-one"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", name, got, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "EDSDK.dll"), []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if again, err := unpackSDK(base, []byte("sdk-one"), []byte("image-one")); err != nil || again != dir {
		t.Fatalf("repair: %s %v", again, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "EDSDK.dll")); string(got) != "sdk-one" {
		t.Fatal("corrupt file not repaired")
	}
	other, err := unpackSDK(base, []byte("sdk-two"), []byte("image-one"))
	if err != nil || other == dir {
		t.Fatalf("version isolation: %s %v", other, err)
	}
	if _, err := unpackSDK(base, nil, []byte("image")); err == nil {
		t.Fatal("incomplete SDK accepted")
	}
}
