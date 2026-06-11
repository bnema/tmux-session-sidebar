package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureRuntimeDirPrivate_CreatesMissingDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "runtime")

	if err := EnsureRuntimeDirPrivate(dir); err != nil {
		t.Fatalf("EnsureRuntimeDirPrivate for missing dir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat created dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("path is not a directory")
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("permissions = %o, want 0700", info.Mode().Perm())
	}
}

func TestEnsureRuntimeDirPrivate_RejectsPermissiveExistingDir(t *testing.T) {
	dir := t.TempDir()

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod 0755: %v", err)
	}

	if err := EnsureRuntimeDirPrivate(dir); err == nil {
		t.Fatal("EnsureRuntimeDirPrivate for permissive dir: got nil, want error")
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat after reject: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("permissions = %o, want unchanged 0755", info.Mode().Perm())
	}
}

func TestEnsureRuntimeDirPrivate_RejectsNonDirectory(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "not-a-dir")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := EnsureRuntimeDirPrivate(path); err == nil {
		t.Fatal("EnsureRuntimeDirPrivate on a file: got nil, want error")
	}
}

func TestEnsureRuntimeDirPrivate_ExistingPrivateDirIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod 0700: %v", err)
	}
	if err := EnsureRuntimeDirPrivate(dir); err != nil {
		t.Fatalf("EnsureRuntimeDirPrivate for private dir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("permissions = %o, want 0700", info.Mode().Perm())
	}
}

func TestEnsureRuntimeDirPrivate_RejectsSymlinkDir(t *testing.T) {
	// Create the target with private permissions so we can verify they stay intact.
	target, err := os.MkdirTemp("", "ensure-runtime-target-*")
	if err != nil {
		t.Fatalf("MkdirTemp target: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(target); err != nil {
			t.Fatalf("cleanup target: %v", err)
		}
	}()
	if err := os.Chmod(target, 0o700); err != nil {
		t.Fatalf("chmod target 0700: %v", err)
	}

	symDir := t.TempDir()
	symPath := filepath.Join(symDir, "mylink")
	if err := os.Symlink(target, symPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := EnsureRuntimeDirPrivate(symPath); err == nil {
		t.Fatal("EnsureRuntimeDirPrivate on symlink: got nil, want error")
	}
	// The target directory must be left untouched.
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target after reject: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("target permissions changed from 0700 to %o after rejected chmod", info.Mode().Perm())
	}
}

func TestEnsureRuntimeDirPrivate_RejectsSymlinkFile(t *testing.T) {
	targetFile := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(targetFile, []byte("data"), 0o600); err != nil {
		t.Fatalf("write target file: %v", err)
	}
	symPath := filepath.Join(t.TempDir(), "filelink")
	if err := os.Symlink(targetFile, symPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := EnsureRuntimeDirPrivate(symPath); err == nil {
		t.Fatal("EnsureRuntimeDirPrivate on symlink to file: got nil, want error")
	}
}

func TestEnsureRuntimeDirPrivate_AcceptsOwnedDir(t *testing.T) {
	// Verify that the ownership check passes for a directory the current user owns.
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod 0700: %v", err)
	}
	if err := EnsureRuntimeDirPrivate(dir); err != nil {
		t.Fatalf("EnsureRuntimeDirPrivate for owned dir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("permissions = %o, want 0700", info.Mode().Perm())
	}
}
