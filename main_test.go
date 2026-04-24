package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseTaskInput(t *testing.T) {
	t.Run("splits on pipe without requiring spaces", func(t *testing.T) {
		text, date := parseTaskInput("Write report|Tomorrow")
		if text != "Write report" || date != "Tomorrow" {
			t.Fatalf("unexpected parse result: text=%q date=%q", text, date)
		}
	})

	t.Run("trims surrounding spaces", func(t *testing.T) {
		text, date := parseTaskInput("  Review notes   |  Apr 30  ")
		if text != "Review notes" || date != "Apr 30" {
			t.Fatalf("unexpected parse result: text=%q date=%q", text, date)
		}
	})

	t.Run("leaves date empty when omitted", func(t *testing.T) {
		text, date := parseTaskInput("Deep work")
		if text != "Deep work" || date != "" {
			t.Fatalf("unexpected parse result: text=%q date=%q", text, date)
		}
	})
}

func TestSaveAndLoadStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	state := AppState{
		Quadrants: [4][]Task{
			{
				{Text: "Deep work", Date: "Apr 24", Completed: true},
			},
			nil,
			{
				{Text: "Inbox cleanup"},
			},
			nil,
		},
	}

	if err := saveStateFile(path, state); err != nil {
		t.Fatalf("saveStateFile() error = %v", err)
	}

	loaded, ok, err := loadStateFile(path)
	if err != nil {
		t.Fatalf("loadStateFile() error = %v", err)
	}
	if !ok {
		t.Fatal("loadStateFile() reported missing file for saved state")
	}
	if !reflect.DeepEqual(state, loaded) {
		t.Fatalf("round-trip mismatch:\nwant: %#v\ngot:  %#v", state, loaded)
	}
}

func TestMoveTaskPersistsAndFocusesTargetQuadrant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	model := Model{
		quadrants: defaultQuadrants(),
		statePath: path,
	}

	original := model.quadrants[0].Tasks[0]
	if ok := model.moveTask(0, 0, 1); !ok {
		t.Fatal("moveTask() returned false")
	}

	if model.focusedQuad != 1 {
		t.Fatalf("focusedQuad = %d, want 1", model.focusedQuad)
	}
	if len(model.quadrants[0].Tasks) != 3 {
		t.Fatalf("source quadrant length = %d, want 3", len(model.quadrants[0].Tasks))
	}

	targetTasks := model.quadrants[1].Tasks
	if len(targetTasks) == 0 {
		t.Fatal("target quadrant is empty after move")
	}
	if !reflect.DeepEqual(targetTasks[len(targetTasks)-1], original) {
		t.Fatalf("moved task mismatch: got %#v want %#v", targetTasks[len(targetTasks)-1], original)
	}

	loaded, ok, err := loadStateFile(path)
	if err != nil {
		t.Fatalf("loadStateFile() error = %v", err)
	}
	if !ok {
		t.Fatal("persisted state file not written")
	}
	if got := loaded.Quadrants[1][len(loaded.Quadrants[1])-1]; !reflect.DeepEqual(got, original) {
		t.Fatalf("persisted moved task mismatch: got %#v want %#v", got, original)
	}
}
