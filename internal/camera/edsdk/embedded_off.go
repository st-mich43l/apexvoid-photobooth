//go:build !windows || !embedcanon

package edsdk

func bundledSDK() ([]byte, []byte) { return nil, nil }
