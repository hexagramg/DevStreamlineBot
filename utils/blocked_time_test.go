package utils

import (
	"testing"
	"time"

	"devstreamlinebot/models"
	"devstreamlinebot/testutils"
)

// Tests for CalculateBlockedTime

func TestCalculateBlockedTime_NoActions(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	start := time.Now().Add(-5 * time.Hour)
	end := time.Now()

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)
	if blocked != 0 {
		t.Errorf("CalculateBlockedTime() = %v, want 0 (no actions)", blocked)
	}
}

func TestCalculateBlockedTime_SingleBlockPeriod(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	// Window: T to T+5h
	// Block label added at T+1h, removed at T+3h
	// Expected blocked: 2h
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // Monday 9am
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", baseTime.Add(1*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", baseTime.Add(3*time.Hour))

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)
	expected := 2 * time.Hour

	if blocked != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v", blocked, expected)
	}
}

func TestCalculateBlockedTime_BlockedAtWindowStart(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	// Block label added BEFORE window start, removed at T+2h
	// Window: T to T+5h
	// Expected blocked: 2h (from window start to removal)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // Monday 9am
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	// Block label added 1 hour before window starts
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", baseTime.Add(-1*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", baseTime.Add(2*time.Hour))

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)
	expected := 2 * time.Hour

	if blocked != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v", blocked, expected)
	}
}

func TestCalculateBlockedTime_StillBlockedAtEnd(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	// Block label added at T+1h, never removed
	// Window: T to T+5h
	// Expected blocked: 4h (from T+1h to end)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // Monday 9am
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", baseTime.Add(1*time.Hour))

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)
	expected := 4 * time.Hour

	if blocked != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v", blocked, expected)
	}
}

func TestCalculateBlockedTime_MultipleOverlappingLabels(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	// Label A added at T+1h
	// Label B added at T+2h
	// Label A removed at T+3h
	// Label B removed at T+4h
	// Window: T to T+5h
	// Expected blocked: 3h (T+1h to T+4h, NOT 2h+2h=4h)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // Monday 9am
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked-a", baseTime.Add(1*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked-b", baseTime.Add(2*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked-a", baseTime.Add(3*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked-b", baseTime.Add(4*time.Hour))

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)
	expected := 3 * time.Hour

	if blocked != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v (overlapping should count once)", blocked, expected)
	}
}

func TestCalculateBlockedTime_MultipleNonOverlapping(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	// Label A added at T+1h, removed at T+2h (1h blocked)
	// Label B added at T+3h, removed at T+4h (1h blocked)
	// Window: T to T+5h
	// Expected blocked: 2h (1h + 1h)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // Monday 9am
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked-a", baseTime.Add(1*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked-a", baseTime.Add(2*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked-b", baseTime.Add(3*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked-b", baseTime.Add(4*time.Hour))

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)
	expected := 2 * time.Hour

	if blocked != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v", blocked, expected)
	}
}

func TestCalculateBlockedTime_BlockedBeforeAndAfterWindow(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	// Block label added at T-2h (before window), removed at T+10h (after window end at T+5h)
	// Window: T to T+5h
	// Expected blocked: 5h (entire window is blocked)
	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // Monday 9am
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", baseTime.Add(-2*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", baseTime.Add(10*time.Hour))

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)
	expected := 5 * time.Hour

	if blocked != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v", blocked, expected)
	}
}

func TestCalculateBlockedTime_ExcludesWeekends(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	// Block period spans Friday 4pm to Monday 10am
	// CalculateWorkingTime counts all hours on working days (24h per day)
	// Friday: 4pm to midnight = 8h (working day)
	// Saturday: 0h (weekend excluded)
	// Sunday: 0h (weekend excluded)
	// Monday: midnight to 10am = 10h (working day)
	// Expected: 18h total working time blocked
	fridayStart := time.Date(2024, 1, 12, 16, 0, 0, 0, time.UTC) // Friday 4pm
	mondayEnd := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)   // Monday 10am

	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", fridayStart)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", mondayEnd)

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, fridayStart, mondayEnd)

	// Friday 4pm-midnight = 8h, Monday midnight-10am = 10h, total = 18h
	expected := 18 * time.Hour

	if blocked != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v (should exclude weekends)", blocked, expected)
	}
}

// Tests for IsMRBlocked

func TestIsMRBlocked_NoLabels(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	// Configure a block label for the repo
	testutils.CreateBlockLabel(db, repo, "blocked")

	// MR has no labels
	if IsMRBlocked(db, &mr) {
		t.Error("IsMRBlocked() = true, want false (MR has no labels)")
	}
}

func TestIsMRBlocked_NoBlockLabelsConfigured(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "feature", "urgent"))

	// No block labels configured for repo

	if IsMRBlocked(db, &mr) {
		t.Error("IsMRBlocked() = true, want false (no block labels configured)")
	}
}

func TestIsMRBlocked_HasBlockLabel(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "blocked"))

	// Configure "blocked" as a block label
	testutils.CreateBlockLabel(db, repo, "blocked")

	if !IsMRBlocked(db, &mr) {
		t.Error("IsMRBlocked() = false, want true (MR has block label)")
	}
}

func TestIsMRBlocked_MixedLabels(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "feature", "blocked", "urgent"))

	// Only "blocked" is configured as a block label
	testutils.CreateBlockLabel(db, repo, "blocked")

	if !IsMRBlocked(db, &mr) {
		t.Error("IsMRBlocked() = false, want true (MR has block label among other labels)")
	}
}

func TestIsMRBlocked_DifferentRepo(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repoA := repoFactory.Create()
	repoB := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repoA, author, testutils.WithLabels(db, "blocked"))

	// "blocked" is only configured as a block label for repo B, not repo A
	testutils.CreateBlockLabel(db, repoB, "blocked")

	if IsMRBlocked(db, &mr) {
		t.Error("IsMRBlocked() = true, want false (block label is for different repo)")
	}
}

func TestIsMRBlocked_MultipleBlockLabels(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author, testutils.WithLabels(db, "on-hold"))

	// Configure multiple block labels
	testutils.CreateBlockLabel(db, repo, "blocked")
	testutils.CreateBlockLabel(db, repo, "on-hold")
	testutils.CreateBlockLabel(db, repo, "waiting-external")

	// MR has "on-hold" which is one of the block labels
	if !IsMRBlocked(db, &mr) {
		t.Error("IsMRBlocked() = false, want true (MR has one of multiple configured block labels)")
	}
}

// ============================================================================
// Edge Case Tests for CalculateBlockedTime
// ============================================================================

// 1. Window Boundary Edge Cases

func TestCalculateBlockedTime_EmptyWindow(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	// Add block action (should not matter for empty window)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", baseTime)

	// Empty window: start == end
	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, baseTime, baseTime)

	if blocked != 0 {
		t.Errorf("CalculateBlockedTime() = %v, want 0 (empty window)", blocked)
	}
}

func TestCalculateBlockedTime_NegativeWindow(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	// Add block action
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", baseTime)

	// Negative window: end < start
	start := baseTime.Add(5 * time.Hour)
	end := baseTime

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)

	if blocked != 0 {
		t.Errorf("CalculateBlockedTime() = %v, want 0 (negative window)", blocked)
	}
}

func TestCalculateBlockedTime_ActionsExactlyAtBoundaries(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC) // Monday 9am
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	// Block added exactly at window start, removed exactly at window end
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", start)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", end)

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)
	expected := 5 * time.Hour

	if blocked != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v (full window blocked)", blocked, expected)
	}
}

// 2. Action Timing Edge Cases

func TestCalculateBlockedTime_ZeroDurationBlock(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	// Block added and removed at the exact same timestamp
	sameTime := baseTime.Add(2 * time.Hour)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", sameTime)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", sameTime)

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)

	if blocked != 0 {
		t.Errorf("CalculateBlockedTime() = %v, want 0 (zero-duration block)", blocked)
	}
}

func TestCalculateBlockedTime_ActionsOnlyBeforeWindow(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	// All actions occur before window start (complete block that ended before window)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", baseTime.Add(-3*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", baseTime.Add(-1*time.Hour))

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)

	if blocked != 0 {
		t.Errorf("CalculateBlockedTime() = %v, want 0 (all actions before window)", blocked)
	}
}

func TestCalculateBlockedTime_ActionsOnlyAfterWindow(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	// All actions occur after window end
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", end.Add(1*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", end.Add(3*time.Hour))

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)

	if blocked != 0 {
		t.Errorf("CalculateBlockedTime() = %v, want 0 (all actions after window)", blocked)
	}
}

func TestCalculateBlockedTime_RapidToggling(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	// Multiple rapid add/remove cycles
	// T+1h: add, T+1h30m: remove (30m)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", baseTime.Add(1*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", baseTime.Add(1*time.Hour+30*time.Minute))

	// T+2h: add, T+2h30m: remove (30m)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", baseTime.Add(2*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", baseTime.Add(2*time.Hour+30*time.Minute))

	// T+3h: add, T+3h30m: remove (30m)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", baseTime.Add(3*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", baseTime.Add(3*time.Hour+30*time.Minute))

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)
	expected := 90 * time.Minute // 3 × 30m = 1.5h

	if blocked != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v (rapid toggling)", blocked, expected)
	}
}

// 3. Data Integrity Edge Cases

func TestCalculateBlockedTime_OrphanedRemoveAction(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	// Remove action without corresponding add action (orphaned/inconsistent data)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", baseTime.Add(2*time.Hour))

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)

	// Should return 0 and not crash
	if blocked != 0 {
		t.Errorf("CalculateBlockedTime() = %v, want 0 (orphaned remove action)", blocked)
	}
}

func TestCalculateBlockedTime_MultipleAddsWithoutRemoves(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	// Multiple add actions in sequence without removes (redundant adds)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked-a", baseTime.Add(1*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked-b", baseTime.Add(2*time.Hour))
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked-c", baseTime.Add(3*time.Hour))

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, start, end)
	// Blocked from first add (T+1h) to window end (T+5h) = 4h
	expected := 4 * time.Hour

	if blocked != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v (multiple adds without removes)", blocked, expected)
	}
}

func TestCalculateBlockedTime_DifferentMRIsolation(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr1 := mrFactory.Create(repo, author)
	mr2 := mrFactory.Create(repo, author)

	baseTime := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	start := baseTime
	end := baseTime.Add(5 * time.Hour)

	// MR1: blocked for 2 hours
	testutils.CreateBlockLabelAction(db, mr1, models.ActionBlockLabelAdded, "blocked", baseTime.Add(1*time.Hour))
	testutils.CreateBlockLabelAction(db, mr1, models.ActionBlockLabelRemoved, "blocked", baseTime.Add(3*time.Hour))

	// MR2: blocked for 1 hour
	testutils.CreateBlockLabelAction(db, mr2, models.ActionBlockLabelAdded, "blocked", baseTime.Add(2*time.Hour))
	testutils.CreateBlockLabelAction(db, mr2, models.ActionBlockLabelRemoved, "blocked", baseTime.Add(3*time.Hour))

	blocked1 := CalculateBlockedTime(db, mr1.ID, repo.ID, start, end)
	blocked2 := CalculateBlockedTime(db, mr2.ID, repo.ID, start, end)

	if blocked1 != 2*time.Hour {
		t.Errorf("CalculateBlockedTime(MR1) = %v, want %v", blocked1, 2*time.Hour)
	}
	if blocked2 != 1*time.Hour {
		t.Errorf("CalculateBlockedTime(MR2) = %v, want %v", blocked2, 1*time.Hour)
	}
}

// 4. Holiday Integration Edge Cases

func TestCalculateBlockedTime_BlockSpansHoliday(t *testing.T) {
	db := testutils.SetupTestDB(t)

	repoFactory := testutils.NewRepositoryFactory(db)
	userFactory := testutils.NewUserFactory(db)
	mrFactory := testutils.NewMergeRequestFactory(db)

	repo := repoFactory.Create()
	author := userFactory.Create()
	mr := mrFactory.Create(repo, author)

	// Create a holiday on Wednesday Jan 17, 2024
	holidayDate := time.Date(2024, 1, 17, 0, 0, 0, 0, time.UTC)
	testutils.CreateHoliday(db, repo, holidayDate)

	// Block period spans Monday through Thursday (includes Wednesday holiday)
	// Monday 9am to Thursday 5pm
	mondayStart := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)   // Monday 9am
	thursdayEnd := time.Date(2024, 1, 18, 17, 0, 0, 0, time.UTC)  // Thursday 5pm

	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelAdded, "blocked", mondayStart)
	testutils.CreateBlockLabelAction(db, mr, models.ActionBlockLabelRemoved, "blocked", thursdayEnd)

	blocked := CalculateBlockedTime(db, mr.ID, repo.ID, mondayStart, thursdayEnd)

	// Expected: Mon (15h) + Tue (24h) + Wed (0h - holiday) + Thu (17h) = 56h
	// Monday 9am to midnight = 15h
	// Tuesday = 24h
	// Wednesday = 0h (holiday excluded)
	// Thursday midnight to 5pm = 17h
	expected := 56 * time.Hour

	if blocked != expected {
		t.Errorf("CalculateBlockedTime() = %v, want %v (block spans holiday)", blocked, expected)
	}
}
