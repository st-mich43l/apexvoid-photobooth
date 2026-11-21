//go:build windows

package settings

import "golang.org/x/sys/windows"

// documentsDir asks Windows for the real Documents folder
// (SHGetKnownFolderPath(FOLDERID_Documents)). It follows OneDrive/Known
// Folder redirection and works with non-ASCII user names; USERPROFILE\Documents
// is never assumed.
func documentsDir() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DEFAULT)
}
