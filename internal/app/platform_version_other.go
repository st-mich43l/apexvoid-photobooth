//go:build !windows

package app

func platformVersion() string { return "unavailable" }
