package balancer

import (
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestSuccessBoost_Select(t *testing.T) {
	items := []model.GroupItem{
		{ID: 1, Priority: 10, Weight: 1},
		{ID: 2, Priority: 5, Weight: 1},
		{ID: 3, Priority: 20, Weight: 1},
	}

	b := &SuccessBoost{}

	// Should select the one with lowest priority value (5)
	selected := b.Select(items)
	if selected == nil {
		t.Fatal("expected selected item, got nil")
	}
	if selected.ID != 2 {
		t.Errorf("expected ID 2 (Priority 5), got ID %d (Priority %d)", selected.ID, selected.Priority)
	}
}

func TestSuccessBoost_Next(t *testing.T) {
	items := []model.GroupItem{
		{ID: 1, Priority: 10, Weight: 1},
		{ID: 2, Priority: 5, Weight: 1},
		{ID: 3, Priority: 20, Weight: 1},
	}

	b := &SuccessBoost{}

	// Current is ID 2 (Priority 5). Next should be ID 1 (Priority 10)
	current := &items[1] // ID 2
	next := b.Next(items, current)

	if next == nil {
		t.Fatal("expected next item, got nil")
	}
	if next.ID != 1 {
		t.Errorf("expected ID 1 (Priority 10), got ID %d (Priority %d)", next.ID, next.Priority)
	}

	// Current is ID 1 (Priority 10). Next should be ID 3 (Priority 20)
	current = &items[0] // ID 1
	next = b.Next(items, current)

	if next == nil {
		t.Fatal("expected next item, got nil")
	}
	if next.ID != 3 {
		t.Errorf("expected ID 3 (Priority 20), got ID %d (Priority %d)", next.ID, next.Priority)
	}

	// Current is ID 3 (Priority 20). Next should be nil
	current = &items[2] // ID 3
	next = b.Next(items, current)

	if next != nil {
		t.Errorf("expected nil (end of list), got ID %d", next.ID)
	}
}

func TestGetBalancer(t *testing.T) {
	b := GetBalancer(model.GroupModeSuccessBoost)
	if _, ok := b.(*SuccessBoost); !ok {
		t.Error("GetBalancer(GroupModeSuccessBoost) should return *SuccessBoost")
	}
}
