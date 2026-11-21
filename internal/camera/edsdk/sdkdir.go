package edsdk

import (
	"debug/pe"
	"fmt"
	"os"
	"path/filepath"
)

// sdkCandidates lists directories searched for EDSDK.dll, in order: the
// configured directory, then locations relative to the executable (portable
// install) including Canon's own package layout.
func sdkCandidates(configured, exeDir string) []string {
	var c []string
	if configured != "" {
		c = append(c, configured, filepath.Join(configured, "Dll"), filepath.Join(configured, "EDSDK_64", "Dll"))
	}
	if exeDir != "" {
		c = append(c,
			exeDir,
			filepath.Join(exeDir, "sdk", "canon"),
			filepath.Join(exeDir, "sdk", "canon", "Dll"),
			filepath.Join(exeDir, "EDSDK_64", "Dll"),
			filepath.Join(exeDir, "EDSDK", "Dll"),
		)
	}
	return c
}

// locateSDK returns the full path of EDSDK.dll and verifies that it is a
// 64-bit DLL with EdsImage.dll beside it.
func locateSDK(configured, exeDir string) (string, error) {
	var last error
	for _, d := range sdkCandidates(configured, exeDir) {
		p := filepath.Join(d, "EDSDK.dll")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := checkX64(p); err != nil {
			last = err
			continue
		}
		if _, err := os.Stat(filepath.Join(d, "EdsImage.dll")); err != nil {
			last = fmt.Errorf("EdsImage.dll is missing next to %s", p)
			continue
		}
		return p, nil
	}
	if last != nil {
		return "", last
	}
	return "", fmt.Errorf("EDSDK.dll not found (searched %d locations)", len(sdkCandidates(configured, exeDir)))
}

// checkX64 verifies the PE machine type: a 32-bit EDSDK.dll cannot be loaded
// by this 64-bit application (Canon ships both; use the EDSDK_64 folder).
func checkX64(path string) error {
	f, err := pe.Open(path)
	if err != nil {
		return fmt.Errorf("%s is not a valid DLL: %w", path, err)
	}
	defer f.Close()
	if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return fmt.Errorf("%s is not a 64-bit DLL (machine 0x%x); use the EDSDK_64 files", path, f.Machine)
	}
	return nil
}
