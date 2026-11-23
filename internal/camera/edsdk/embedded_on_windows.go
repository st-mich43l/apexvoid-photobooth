//go:build windows && embedcanon

package edsdk

import _ "embed"

// SDK files are supplied locally at build time, never committed to this repo.
//
//go:embed embedded/EDSDK.dll
var bundledEDSDK []byte

//go:embed embedded/EdsImage.dll
var bundledEdsImage []byte

func bundledSDK() ([]byte, []byte) { return bundledEDSDK, bundledEdsImage }
