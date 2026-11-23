package storage

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOwnershipAcrossProcesses(t *testing.T) {
	root := t.TempDir()
	owner, err := AcquireOwnership(root)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnershipHelper$")
	cmd.Env = append(os.Environ(), "APEXVOID_LOCK_TEST="+root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("competing process: %v %s", err, out)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := AcquireOwnership(filepath.Join(root, "."))
	if err != nil {
		t.Fatal(err)
	}
	_ = next.Close()
}
func TestOwnershipHelper(t *testing.T) {
	root := os.Getenv("APEXVOID_LOCK_TEST")
	if root == "" {
		return
	}
	owner, err := AcquireOwnership(root)
	if owner != nil {
		_ = owner.Close()
	}
	if !errors.Is(err, ErrLibraryOwned) {
		t.Fatalf("expected ownership error, got %v", err)
	}
}
func TestOwnershipReleasedAfterProcessExit(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnershipCrashHelper$")
	cmd.Env = append(os.Environ(), "APEXVOID_LOCK_CRASH="+root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper: %v %s", err, out)
	}
	owner, err := AcquireOwnership(root)
	if err != nil {
		t.Fatal(err)
	}
	_ = owner.Close()
}
func TestOwnershipCrashHelper(t *testing.T) {
	root := os.Getenv("APEXVOID_LOCK_CRASH")
	if root == "" {
		return
	}
	if _, err := AcquireOwnership(root); err != nil {
		t.Fatal(err)
	}
	os.Exit(0) // emulate process death without closing the ownership handle
}
