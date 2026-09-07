package core

import (
	"strings"
	"testing"
)

func TestFindObsScriptsDir(t *testing.T) {
	dir, err := FindObsScriptsDir()
	if err != nil {
		t.Fatalf("FindObsScriptsDir failed: %v", err)
	}

	if dir == "" {
		t.Fatalf("FindObsScriptsDir returned empty path")
	}

	if !strings.Contains(dir, "obs") {
		t.Errorf("expected OBS path, got: %s", dir)
	}
}

func TestFindRunningObsPath(t *testing.T) {
	// Should run without panic or memory error
	_ = FindRunningObsPath()
}

func TestDetectObsScriptsDir(t *testing.T) {
	path, detected := DetectObsScriptsDir()
	if path == "" {
		t.Fatalf("DetectObsScriptsDir returned empty path")
	}
	t.Logf("Detected: %v, Path: %s", detected, path)
}

func TestResolveObsScriptsDir(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    `D:\OBS\obs-studio\data\obs-plugins\frontend-tools\scripts`,
			expected: `D:\OBS\obs-studio\data\obs-plugins\frontend-tools\scripts`,
		},
		{
			input:    `D:\Custom\obs-studio\bin\64bit`,
			expected: `D:\Custom\obs-studio\data\obs-plugins\frontend-tools\scripts`,
		},
		{
			input:    `E:\Tools\obs-studio`,
			expected: `E:\Tools\obs-studio\data\obs-plugins\frontend-tools\scripts`,
		},
	}

	for _, tc := range tests {
		got := ResolveObsScriptsDir(tc.input)
		if !strings.EqualFold(got, tc.expected) {
			t.Errorf("ResolveObsScriptsDir(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestIsScriptInstalled(t *testing.T) {
	// Should return boolean without crashing
	_ = IsScriptInstalled("")
}

func TestPrepareLauncherScriptWithExePath(t *testing.T) {
	mockScript := []byte(`
local BAKED_EXE_PATH = ""
print("test")
`)
	result := PrepareLauncherScriptWithExePath(mockScript)
	resultStr := string(result)

	if strings.Contains(resultStr, `local BAKED_EXE_PATH = ""`) {
		t.Errorf("expected BAKED_EXE_PATH to be replaced, got: %s", resultStr)
	}

	if !strings.Contains(resultStr, `local BAKED_EXE_PATH = "`) {
		t.Errorf("expected baked path in script, got: %s", resultStr)
	}
}


