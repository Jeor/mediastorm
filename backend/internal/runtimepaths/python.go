package runtimepaths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Python returns the packaged or development Python interpreter. Explicit
// overrides take priority so launchers and service wrappers do not need to
// change the process working directory.
func Python() (string, error) {
	if override := strings.TrimSpace(os.Getenv("STRMR_PYTHON")); override != "" {
		if isFile(override) {
			return override, nil
		}
		return "", fmt.Errorf("STRMR_PYTHON does not point to a file: %s", override)
	}

	var candidates []string
	if executableDir, err := executableDirectory(); err == nil {
		candidates = append(candidates,
			filepath.Join(executableDir, "python", "python.exe"),
			filepath.Join(executableDir, "..", "python", "python.exe"),
			filepath.Join(executableDir, "python", "bin", "python3"),
			filepath.Join(executableDir, "..", "python", "bin", "python3"),
		)
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			filepath.Join(".venv", "Scripts", "python.exe"),
			filepath.Join("..", ".venv", "Scripts", "python.exe"),
		)
	} else {
		candidates = append(candidates,
			"/.venv/bin/python3",
			filepath.Join(".venv", "bin", "python3"),
			filepath.Join("..", ".venv", "bin", "python3"),
		)
	}
	return firstFile("Python interpreter", candidates)
}

// PythonScript returns a named helper script from the packaged scripts
// directory, Docker layout, or a source checkout.
func PythonScript(name string) (string, error) {
	if filepath.Base(name) != name || strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("invalid Python script name %q", name)
	}

	var candidates []string
	if scriptsDir := strings.TrimSpace(os.Getenv("STRMR_SCRIPTS_DIR")); scriptsDir != "" {
		candidates = append(candidates, filepath.Join(scriptsDir, name))
	}
	if executableDir, err := executableDirectory(); err == nil {
		candidates = append(candidates,
			filepath.Join(executableDir, "scripts", name),
			filepath.Join(executableDir, "..", "scripts", name),
		)
	}
	candidates = append(candidates,
		filepath.Join(string(filepath.Separator), name),
		filepath.Join("backend", name),
		name,
		filepath.Join("..", name),
	)
	return firstFile("Python script "+name, candidates)
}

func executableDirectory() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err == nil {
		executable = resolved
	}
	return filepath.Dir(executable), nil
}

func firstFile(label string, candidates []string) (string, error) {
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if isFile(candidate) {
			absolute, err := filepath.Abs(candidate)
			if err == nil {
				return absolute, nil
			}
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s not found", label)
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
