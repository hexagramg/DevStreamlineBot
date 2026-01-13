package utils

import (
	"testing"

	"devstreamlinebot/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestFormatSLAFromDigest_NotBlocked(t *testing.T) {
	tests := []struct {
		name     string
		dmr      DigestMR
		expected string
	}{
		{
			name: "Zero percentage",
			dmr: DigestMR{
				SLAPercentage: 0,
				SLAExceeded:   false,
				Blocked:       false,
			},
			expected: "N/A",
		},
		{
			name: "Normal percentage",
			dmr: DigestMR{
				SLAPercentage: 50,
				SLAExceeded:   false,
				Blocked:       false,
			},
			expected: "50%",
		},
		{
			name: "Warning percentage (80%+)",
			dmr: DigestMR{
				SLAPercentage: 85,
				SLAExceeded:   false,
				Blocked:       false,
			},
			expected: "85% ⚠️",
		},
		{
			name: "Exceeded percentage",
			dmr: DigestMR{
				SLAPercentage: 120,
				SLAExceeded:   true,
				Blocked:       false,
			},
			expected: "120% ❌",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSLAFromDigest(&tt.dmr)
			if result != tt.expected {
				t.Errorf("formatSLAFromDigest() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatSLAFromDigest_BlockedWithPercentage(t *testing.T) {
	dmr := DigestMR{
		SLAPercentage: 50,
		SLAExceeded:   false,
		Blocked:       true,
	}

	result := formatSLAFromDigest(&dmr)
	expected := "50% ⏸"

	if result != expected {
		t.Errorf("formatSLAFromDigest() = %q, want %q", result, expected)
	}
}

func TestFormatSLAFromDigest_BlockedWithWarning(t *testing.T) {
	dmr := DigestMR{
		SLAPercentage: 90,
		SLAExceeded:   false,
		Blocked:       true,
	}

	result := formatSLAFromDigest(&dmr)
	expected := "90% ⚠️ ⏸"

	if result != expected {
		t.Errorf("formatSLAFromDigest() = %q, want %q", result, expected)
	}
}

func TestFormatSLAFromDigest_BlockedExceeded(t *testing.T) {
	dmr := DigestMR{
		SLAPercentage: 150,
		SLAExceeded:   true,
		Blocked:       true,
	}

	result := formatSLAFromDigest(&dmr)
	expected := "150% ❌ ⏸"

	if result != expected {
		t.Errorf("formatSLAFromDigest() = %q, want %q", result, expected)
	}
}

func TestFormatSLAFromDigest_BlockedNA(t *testing.T) {
	dmr := DigestMR{
		SLAPercentage: 0,
		SLAExceeded:   false,
		Blocked:       true,
	}

	result := formatSLAFromDigest(&dmr)
	expected := "N/A ⏸"

	if result != expected {
		t.Errorf("formatSLAFromDigest() = %q, want %q", result, expected)
	}
}

func TestSanitizeTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Simple title", "Simple title"},
		{"Title\nwith\nnewlines", "Title with newlines"},
		{"Title\r\nwith\r\nCRLF", "Title with CRLF"},
		{"Title  with   extra   spaces", "Title with extra spaces"},
		{"  Trimmed  ", "Trimmed"},
		{"Multiple\n\nNewlines", "Multiple Newlines"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeTitle(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeTitle(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBuildReviewDigest_Empty(t *testing.T) {
	result := BuildReviewDigest(nil, []models.MergeRequest{})

	if result != "No pending reviews found." {
		t.Errorf("BuildReviewDigest() = %q, want %q", result, "No pending reviews found.")
	}
}

func TestBuildEnhancedReviewDigest_Empty(t *testing.T) {
	result := BuildEnhancedReviewDigest(nil, []DigestMR{})

	if result != "No pending reviews found." {
		t.Errorf("BuildEnhancedReviewDigest() = %q, want %q", result, "No pending reviews found.")
	}
}

func TestBuildUserActionsDigest_Empty(t *testing.T) {
	result := BuildUserActionsDigest(nil, []DigestMR{}, []DigestMR{}, "testuser")

	expected := "No pending actions for testuser."
	if result != expected {
		t.Errorf("BuildUserActionsDigest() = %q, want %q", result, expected)
	}
}

func TestBatchGetUserMentions_WithEmails(t *testing.T) {
	db := setupMentionTestDB(t)

	// Create users with emails
	user1 := models.User{GitlabID: 1, Username: "alice", Email: "alice@example.com"}
	user2 := models.User{GitlabID: 2, Username: "bob", Email: "bob@example.com"}
	db.Create(&user1)
	db.Create(&user2)

	result := BatchGetUserMentions(db, []models.User{user1, user2})

	if result[user1.ID] != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %s", result[user1.ID])
	}
	if result[user2.ID] != "bob@example.com" {
		t.Errorf("expected bob@example.com, got %s", result[user2.ID])
	}
}

func TestBatchGetUserMentions_WithVKUsers(t *testing.T) {
	db := setupMentionTestDB(t)

	// Create users without emails
	user1 := models.User{GitlabID: 1, Username: "alice", Email: ""}
	user2 := models.User{GitlabID: 2, Username: "bob", Email: ""}
	db.Create(&user1)
	db.Create(&user2)

	// Create VK users that match by username prefix
	vk1 := models.VKUser{UserID: "alice@vkteams.com", FirstName: "Alice", LastName: "Test"}
	vk2 := models.VKUser{UserID: "bob@vkteams.com", FirstName: "Bob", LastName: "Test"}
	db.Create(&vk1)
	db.Create(&vk2)

	result := BatchGetUserMentions(db, []models.User{user1, user2})

	if result[user1.ID] != "alice@vkteams.com" {
		t.Errorf("expected alice@vkteams.com, got %s", result[user1.ID])
	}
	if result[user2.ID] != "bob@vkteams.com" {
		t.Errorf("expected bob@vkteams.com, got %s", result[user2.ID])
	}
}

func TestBatchGetUserMentions_Mixed(t *testing.T) {
	db := setupMentionTestDB(t)

	// User with email
	userWithEmail := models.User{GitlabID: 1, Username: "alice", Email: "alice@example.com"}
	// User with VK match
	userWithVK := models.User{GitlabID: 2, Username: "bob", Email: ""}
	// User with neither
	userPlain := models.User{GitlabID: 3, Username: "charlie", Email: ""}
	db.Create(&userWithEmail)
	db.Create(&userWithVK)
	db.Create(&userPlain)

	// Create VK user for bob only
	vkBob := models.VKUser{UserID: "bob@vkteams.com", FirstName: "Bob", LastName: "Test"}
	db.Create(&vkBob)

	result := BatchGetUserMentions(db, []models.User{userWithEmail, userWithVK, userPlain})

	if result[userWithEmail.ID] != "alice@example.com" {
		t.Errorf("userWithEmail: expected alice@example.com, got %s", result[userWithEmail.ID])
	}
	if result[userWithVK.ID] != "bob@vkteams.com" {
		t.Errorf("userWithVK: expected bob@vkteams.com, got %s", result[userWithVK.ID])
	}
	if result[userPlain.ID] != "charlie" {
		t.Errorf("userPlain: expected charlie, got %s", result[userPlain.ID])
	}
}

func TestBatchGetUserMentions_Empty(t *testing.T) {
	db := setupMentionTestDB(t)

	result := BatchGetUserMentions(db, []models.User{})

	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestBatchGetUserMentions_NoVKMatch(t *testing.T) {
	db := setupMentionTestDB(t)

	// Users without email and no VK user match
	user1 := models.User{GitlabID: 1, Username: "alice", Email: ""}
	user2 := models.User{GitlabID: 2, Username: "bob", Email: ""}
	db.Create(&user1)
	db.Create(&user2)

	// Create VK user that does NOT match (different username prefix)
	vkOther := models.VKUser{UserID: "charlie@vkteams.com", FirstName: "Charlie", LastName: "Test"}
	db.Create(&vkOther)

	result := BatchGetUserMentions(db, []models.User{user1, user2})

	// Should fall back to usernames
	if result[user1.ID] != "alice" {
		t.Errorf("expected alice, got %s", result[user1.ID])
	}
	if result[user2.ID] != "bob" {
		t.Errorf("expected bob, got %s", result[user2.ID])
	}
}

// setupMentionTestDB creates an in-memory database for mention tests.
func setupMentionTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(&models.User{}, &models.VKUser{})
	if err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}
