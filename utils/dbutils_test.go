package utils

import (
	"testing"

	"devstreamlinebot/models"
)

func TestIsMRBlockedFromCache_Blocked(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "blocked"},
		{Name: "urgent"},
	}
	blockLabels := map[string]struct{}{
		"blocked": {},
		"wip":     {},
	}

	result := isMRBlockedFromCache(labels, blockLabels)

	if !result {
		t.Error("expected MR to be blocked, got not blocked")
	}
}

func TestIsMRBlockedFromCache_NotBlocked(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "ready-for-review"},
		{Name: "urgent"},
	}
	blockLabels := map[string]struct{}{
		"blocked": {},
		"wip":     {},
	}

	result := isMRBlockedFromCache(labels, blockLabels)

	if result {
		t.Error("expected MR to not be blocked, got blocked")
	}
}

func TestIsMRBlockedFromCache_EmptyLabels(t *testing.T) {
	labels := []models.Label{}
	blockLabels := map[string]struct{}{
		"blocked": {},
		"wip":     {},
	}

	result := isMRBlockedFromCache(labels, blockLabels)

	if result {
		t.Error("expected MR with no labels to not be blocked, got blocked")
	}
}

func TestIsMRBlockedFromCache_EmptyBlockList(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "blocked"},
	}
	blockLabels := map[string]struct{}{}

	result := isMRBlockedFromCache(labels, blockLabels)

	if result {
		t.Error("expected MR to not be blocked when no block labels configured, got blocked")
	}
}

func TestIsMRBlockedFromCache_NilBlockList(t *testing.T) {
	labels := []models.Label{
		{Name: "feature"},
		{Name: "blocked"},
	}

	result := isMRBlockedFromCache(labels, nil)

	if result {
		t.Error("expected MR to not be blocked when block labels map is nil, got blocked")
	}
}

func TestIsMRBlockedFromCache_MultipleBlockingLabels(t *testing.T) {
	labels := []models.Label{
		{Name: "blocked"},
		{Name: "wip"},
	}
	blockLabels := map[string]struct{}{
		"blocked": {},
		"wip":     {},
	}

	result := isMRBlockedFromCache(labels, blockLabels)

	if !result {
		t.Error("expected MR with multiple blocking labels to be blocked, got not blocked")
	}
}
