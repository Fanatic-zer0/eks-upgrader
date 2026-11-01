package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileLogger_EmitAppendsJSONLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	logger, err := NewFileLogger(path)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}

	events := []Event{
		{RunID: "run-1", Cluster: "c1", Region: "r1", Type: EventRunStarted, Message: "run started"},
		{RunID: "run-1", Cluster: "c1", Region: "r1", Type: EventValidationCompleted, Message: "validation ok"},
	}
	for _, e := range events {
		if err := logger.Emit(e); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening audit file: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var count int
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("unmarshaling line %d: %v", count, err)
		}
		if e.RunID != "run-1" {
			t.Fatalf("unexpected runID: %q", e.RunID)
		}
		count++
	}
	if count != len(events) {
		t.Fatalf("expected %d lines, got %d", len(events), count)
	}
}

func TestFileLogger_AppendsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	l1, err := NewFileLogger(path)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	if err := l1.Emit(Event{RunID: "run-1", Type: EventRunStarted}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	l1.Close()

	l2, err := NewFileLogger(path)
	if err != nil {
		t.Fatalf("NewFileLogger (reopen): %v", err)
	}
	if err := l2.Emit(Event{RunID: "run-1", Type: EventRunCompleted}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	l2.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading audit file: %v", err)
	}
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Fatalf("expected 2 lines after reopen+append, got %d", lines)
	}
}
