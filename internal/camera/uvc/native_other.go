//go:build !windows

package uvc

// nativeEnumerator is nil off Windows: Linux enumerates /dev/video* through FFmpeg.
var nativeEnumerator Enumerator
