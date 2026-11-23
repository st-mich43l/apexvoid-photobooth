//go:build !windows

package main

import "log"

func startupError(message string) { log.Print(message) }
