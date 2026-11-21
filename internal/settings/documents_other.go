//go:build !windows

package settings

func documentsDir() (string, error) { return "", errNotWindows }
