package runtimepaths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPythonUsesExplicitOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "python.exe")
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STRMR_PYTHON", path)

	got, err := Python()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(path)
	if got != want {
		t.Fatalf("Python() = %q, want %q", got, want)
	}
}

func TestPythonRejectsMissingOverride(t *testing.T) {
	t.Setenv("STRMR_PYTHON", filepath.Join(t.TempDir(), "missing.exe"))
	if _, err := Python(); err == nil {
		t.Fatal("expected missing STRMR_PYTHON to fail")
	}
}

func TestPythonScriptUsesPackagedDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "detect_credits.py")
	if err := os.WriteFile(path, []byte("# test"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STRMR_SCRIPTS_DIR", dir)

	got, err := PythonScript("detect_credits.py")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(path)
	if got != want {
		t.Fatalf("PythonScript() = %q, want %q", got, want)
	}
}

func TestPythonScriptRejectsTraversal(t *testing.T) {
	if _, err := PythonScript(filepath.Join("..", "secret.py")); err == nil {
		t.Fatal("expected traversing script name to fail")
	}
}
