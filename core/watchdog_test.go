package core

import (
	"testing"
)

func TestIsObsRunning(t *testing.T) {
	// IsObsRunning should execute without panicking or returning system error
	_ = IsObsRunning()
}
