package sorter

import (
	"testing"
)

func TestTopologicalSort(t *testing.T) {
	mods := []string{"modA", "modB", "modC", "modD"}
	deps := []ModDependency{
		{Dependent: "modB", Required: "modA"},
		{Dependent: "modC", Required: "modB"},
	}

	result := topologicalSort(mods, deps)

	pos := make(map[string]int)
	for i, m := range result {
		pos[m] = i
	}

	if pos["modA"] > pos["modB"] {
		t.Errorf("modA must be before modB. Got %v", result)
	}
	if pos["modB"] > pos["modC"] {
		t.Errorf("modB must be before modC. Got %v", result)
	}
	if len(result) != 4 {
		t.Errorf("Expected 4 mods, got %d", len(result))
	}
}

func TestTopologicalSortCycle(t *testing.T) {
	mods := []string{"modA", "modB"}
	deps := []ModDependency{
		{Dependent: "modB", Required: "modA"},
		{Dependent: "modA", Required: "modB"},
	}
	result := topologicalSort(mods, deps)

	// При наличии цикла MinHeap не сможет извлечь ни один узел (у всех indegree > 0).
	if len(result) != 0 {
		t.Errorf("Expected 0 mods (cycle), got %v", result)
	}
}
