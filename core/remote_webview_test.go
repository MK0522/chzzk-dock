package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteWindowState(t *testing.T) {
	// Set temp directory for testing
	tempDir, err := os.MkdirTemp("", "chzzk_remote_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origLocalAppData := os.Getenv("LOCALAPPDATA")
	os.Setenv("LOCALAPPDATA", tempDir)
	defer os.Setenv("LOCALAPPDATA", origLocalAppData)

	// Test default state when file does not exist
	defState := loadRemoteWindowState()
	if defState.Width != 700 || defState.Height != 760 {
		t.Errorf("Expected default width 700, height 760; got %d, %d", defState.Width, defState.Height)
	}
	if defState.Topmost != true {
		t.Errorf("Expected default topmost true; got %v", defState.Topmost)
	}

	// Test save and load
	customState := RemoteWindowState{
		X:       200,
		Y:       300,
		Width:   500,
		Height:  800,
		Topmost: true,
	}
	saveRemoteWindowState(customState)

	loaded := loadRemoteWindowState()
	if loaded.X != 200 || loaded.Y != 300 || loaded.Width != 500 || loaded.Height != 800 || !loaded.Topmost {
		t.Errorf("Loaded state mismatch: %+v", loaded)
	}

	// Ensure file exists at expected path
	expectedFile := filepath.Join(tempDir, "ChzzkObsDock", "remote_window.json")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Errorf("Expected state file at %s does not exist", expectedFile)
	}
}
