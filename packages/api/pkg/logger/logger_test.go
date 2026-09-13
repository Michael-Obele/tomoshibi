package logger

import (
	"testing"
)

func TestInit_DefaultLevel(t *testing.T) {
	Init("info")

	if Log == nil {
		t.Fatal("Log should not be nil after Init")
	}
}

func TestInit_DebugLevel(t *testing.T) {
	Init("debug")

	if Log == nil {
		t.Fatal("Log should not be nil")
	}
}

func TestInit_WarnLevel(t *testing.T) {
	Init("warn")

	if Log == nil {
		t.Fatal("Log should not be nil")
	}
}

func TestInit_ErrorLevel(t *testing.T) {
	Init("error")

	if Log == nil {
		t.Fatal("Log should not be nil")
	}
}

func TestInit_UnknownLevel(t *testing.T) {
	// Unknown level should default to info
	Init("unknown")

	if Log == nil {
		t.Fatal("Log should not be nil")
	}
}

func TestInit_EmptyLevel(t *testing.T) {
	Init("")

	if Log == nil {
		t.Fatal("Log should not be nil")
	}
}

func TestLog_CanLog(t *testing.T) {
	Init("info")

	// These should not panic
	Log.Info("test info message")
	Log.Warn("test warn message")
	Log.Error("test error message")
}
