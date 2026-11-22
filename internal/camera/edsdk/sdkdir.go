package edsdk

import (
	"errors"
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

// locateSDK returns the full path of a usable EDSDK.dll: 64-bit, dependencies
// present and every required export available (see Inspect).
func locateSDK(configured, exeDir string) (string, error) {
	r := Inspect(configured, exeDir)
	if r.OK() {
		return r.Path, nil
	}
	return "", errors.New(r.Problem)
}
