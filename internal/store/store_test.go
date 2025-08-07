package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BTreeMap/PromptPipe/internal/models"
)

func TestDetectDSNType(t *testing.T) {
	tests := []struct {
		name           string
		dsn            string
		expectedDriver string
	}{
		{
			name:           "PostgreSQL DSN with postgres:// scheme",
			dsn:            "postgres://user:password@localhost/dbname",
			expectedDriver: "postgres",
		},
		{
			name:           "PostgreSQL DSN with host= parameter",
			dsn:            "host=localhost user=postgres dbname=test",
			expectedDriver: "postgres",
		},
		{
			name:           "PostgreSQL DSN with multiple key=value pairs",
			dsn:            "user=postgres password=secret dbname=test sslmode=disable",
			expectedDriver: "postgres",
		},
		{
			name:           "PostgreSQL DSN with single key=value pair",
			dsn:            "dbname=test",
			expectedDriver: "postgres",
		},
		{
			name:           "SQLite DSN with absolute file path",
			dsn:            "/var/lib/promptpipe/promptpipe.db",
			expectedDriver: "sqlite3",
		},
		{
			name:           "SQLite DSN with relative path",
			dsn:            "./data/promptpipe.db",
			expectedDriver: "sqlite3",
		},
		{
			name:           "SQLite DSN with .db extension",
			dsn:            "test.db",
			expectedDriver: "sqlite3",
		},
		{
			name:           "SQLite DSN without extension",
			dsn:            "/tmp/database",
			expectedDriver: "sqlite3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detectedDriver := DetectDSNType(tt.dsn)
			if detectedDriver != tt.expectedDriver {
				t.Errorf("DSN detection failed for %q: expected driver %q, got %q", tt.dsn, tt.expectedDriver, detectedDriver)
			}
		})
	}
}

func TestInMemoryStore(t *testing.T) {
	s := NewInMemoryStore()
	r1 := models.Receipt{To: "+123", Status: models.MessageStatusSent, Time: 1}
	if err := s.AddReceipt(r1); err != nil {
		t.Fatalf("AddReceipt failed: %v", err)
	}

	resp1 := models.Response{From: "+123", Body: "response 1", Time: 2}
	if err := s.AddResponse(resp1); err != nil {
		t.Fatalf("AddResponse failed: %v", err)
	}

	receipts, err := s.GetReceipts()
	if err != nil {
		t.Fatalf("GetReceipts failed: %v", err)
	}
	if len(receipts) != 1 || receipts[0].To != r1.To {
		t.Errorf("GetReceipts: expected %+v, got %+v", r1, receipts)
	}

	responses, err := s.GetResponses()
	if err != nil {
		t.Fatalf("GetResponses failed: %v", err)
	}
	if len(responses) != 1 || responses[0].From != resp1.From {
		t.Errorf("GetResponses: expected %+v, got %+v", resp1, responses)
	}

	if err := s.ClearReceipts(); err != nil {
		t.Fatalf("ClearReceipts failed: %v", err)
	}
	receipts, _ = s.GetReceipts()
	if len(receipts) != 0 {
		t.Errorf("ClearReceipts: expected 0 receipts, got %d", len(receipts))
	}

	if err := s.ClearResponses(); err != nil {
		t.Fatalf("ClearResponses failed: %v", err)
	}
	responses, _ = s.GetResponses()
	if len(responses) != 0 {
		t.Errorf("ClearResponses: expected 0 responses, got %d", len(responses))
	}
}

func TestPostgresStore(t *testing.T) {
	dsn := getenvOrSkip(t, "POSTGRES_DSN_TEST") // Example: "postgres://user:pass@host:port/dbname?sslmode=disable"
	if dsn == "" {
		t.Skip("POSTGRES_DSN_TEST not set, skipping PostgresStore tests")
	}

	s, err := NewPostgresStore(WithPostgresDSN(dsn))
	if err != nil {
		t.Fatalf("NewPostgresStore failed: %v", err)
	}
	defer s.Close() // Correctly call Close on s

	// Clear existing data for a clean test run
	if err := s.ClearReceipts(); err != nil {
		// It's okay if the table doesn't exist yet on the first run before migrations
		// t.Logf("ClearReceipts (pre-test) failed, possibly due to no table: %v", err)
	}
	if err := s.ClearResponses(); err != nil {
		// t.Logf("ClearResponses (pre-test) failed, possibly due to no table: %v", err)
	}

	r1 := models.Receipt{To: "pg_test_1", Status: models.MessageStatusDelivered, Time: 100}
	if err := s.AddReceipt(r1); err != nil {
		t.Fatalf("AddReceipt failed: %v", err)
	}

	receipts, err := s.GetReceipts()
	if err != nil {
		t.Fatalf("GetReceipts failed: %v", err)
	}
	foundR1 := false
	for _, r := range receipts {
		if r.To == r1.To && r.Status == r1.Status && r.Time == r1.Time {
			foundR1 = true
			break
		}
	}
	if !foundR1 {
		t.Errorf("GetReceipts: did not find expected receipt %+v in %+v", r1, receipts)
	}

	resp1 := models.Response{From: "pg_test_resp_1", Body: "postgres response", Time: 101}
	if err := s.AddResponse(resp1); err != nil {
		t.Fatalf("AddResponse failed: %v", err)
	}

	responses, err := s.GetResponses()
	if err != nil {
		t.Fatalf("GetResponses failed: %v", err)
	}
	foundResp1 := false
	for _, r := range responses {
		if r.From == resp1.From && r.Body == resp1.Body && r.Time == resp1.Time {
			foundResp1 = true
			break
		}
	}
	if !foundResp1 {
		t.Errorf("GetResponses: did not find expected response %+v in %+v", resp1, responses)
	}

	if err := s.ClearReceipts(); err != nil {
		t.Fatalf("ClearReceipts failed: %v", err)
	}
	receipts, _ = s.GetReceipts()
	if len(receipts) != 0 {
		t.Errorf("ClearReceipts: expected 0 receipts, got %d", len(receipts))
	}

	if err := s.ClearResponses(); err != nil {
		t.Fatalf("ClearResponses failed: %v", err)
	}
	responses, _ = s.GetResponses()
	if len(responses) != 0 {
		t.Errorf("ClearResponses: expected 0 responses, got %d", len(responses))
	}
}

func TestSQLiteStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlite_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	s, err := NewSQLiteStore(WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer s.Close() // Ensure SQLiteStore has a Close method

	r1 := models.Receipt{To: "sqlite_test_1", Status: models.MessageStatusRead, Time: 200}
	if err := s.AddReceipt(r1); err != nil {
		t.Fatalf("AddReceipt failed: %v", err)
	}

	receipts, err := s.GetReceipts()
	if err != nil {
		t.Fatalf("GetReceipts failed: %v", err)
	}
	if len(receipts) != 1 || receipts[0].To != r1.To || receipts[0].Status != r1.Status || receipts[0].Time != r1.Time {
		t.Errorf("GetReceipts: expected %+v, got %+v", r1, receipts)
	}

	resp1 := models.Response{From: "sqlite_test_resp_1", Body: "sqlite response", Time: 201}
	if err := s.AddResponse(resp1); err != nil {
		t.Fatalf("AddResponse failed: %v", err)
	}

	responses, err := s.GetResponses()
	if err != nil {
		t.Fatalf("GetResponses failed: %v", err)
	}
	if len(responses) != 1 || responses[0].From != resp1.From || responses[0].Body != resp1.Body || responses[0].Time != resp1.Time {
		t.Errorf("GetResponses: expected %+v, got %+v", resp1, responses[0])
	}

	if err := s.ClearReceipts(); err != nil {
		t.Fatalf("ClearReceipts failed: %v", err)
	}
	receipts, _ = s.GetReceipts()
	if len(receipts) != 0 {
		t.Errorf("ClearReceipts: expected 0 receipts, got %d", len(receipts))
	}

	if err := s.ClearResponses(); err != nil {
		t.Fatalf("ClearResponses failed: %v", err)
	}
	responses, _ = s.GetResponses()
	if len(responses) != 0 {
		t.Errorf("ClearResponses: expected 0 responses, got %d", len(responses))
	}
}

func TestInMemoryStore_HookPersistence(t *testing.T) {
	s := NewInMemoryStore()

	// Test saving a hook
	hook := models.RegisteredHook{
		PhoneNumber: "+1234567890",
		HookType:    models.HookTypeConversation,
		Parameters: map[string]string{
			"participant_id": "test-participant",
			"phone_number":   "+1234567890",
		},
		CreatedAt: time.Now(),
	}

	err := s.SaveRegisteredHook(hook)
	if err != nil {
		t.Fatalf("SaveRegisteredHook failed: %v", err)
	}

	// Test getting the hook
	retrievedHook, err := s.GetRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("GetRegisteredHook failed: %v", err)
	}

	if retrievedHook == nil {
		t.Fatal("Retrieved hook is nil")
	}

	if retrievedHook.PhoneNumber != hook.PhoneNumber {
		t.Errorf("Expected phone number '%s', got '%s'", hook.PhoneNumber, retrievedHook.PhoneNumber)
	}

	if retrievedHook.HookType != hook.HookType {
		t.Errorf("Expected hook type '%s', got '%s'", hook.HookType, retrievedHook.HookType)
	}

	if retrievedHook.Parameters["participant_id"] != hook.Parameters["participant_id"] {
		t.Errorf("Expected participant_id '%s', got '%s'",
			hook.Parameters["participant_id"], retrievedHook.Parameters["participant_id"])
	}

	// Test listing hooks
	hooks, err := s.ListRegisteredHooks()
	if err != nil {
		t.Fatalf("ListRegisteredHooks failed: %v", err)
	}

	if len(hooks) != 1 {
		t.Errorf("Expected 1 hook, got %d", len(hooks))
	}

	if hooks[0].PhoneNumber != hook.PhoneNumber {
		t.Errorf("Listed hook has wrong phone number: expected '%s', got '%s'",
			hook.PhoneNumber, hooks[0].PhoneNumber)
	}

	// Test deleting the hook
	err = s.DeleteRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("DeleteRegisteredHook failed: %v", err)
	}

	// Verify hook is deleted
	deletedHook, err := s.GetRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("GetRegisteredHook after deletion failed: %v", err)
	}

	if deletedHook != nil {
		t.Error("Hook should have been deleted")
	}

	// Verify list is empty
	hooks, err = s.ListRegisteredHooks()
	if err != nil {
		t.Fatalf("ListRegisteredHooks after deletion failed: %v", err)
	}

	if len(hooks) != 0 {
		t.Errorf("Expected 0 hooks after deletion, got %d", len(hooks))
	}
}

func TestInMemoryStore_HookPersistence_MultipleHooks(t *testing.T) {
	s := NewInMemoryStore()

	// Create multiple hooks
	hook1 := models.RegisteredHook{
		PhoneNumber: "+1111111111",
		HookType:    models.HookTypeConversation,
		Parameters: map[string]string{
			"participant_id": "participant-1",
			"phone_number":   "+1111111111",
		},
		CreatedAt: time.Now(),
	}

	hook2 := models.RegisteredHook{
		PhoneNumber: "+2222222222",
		HookType:    models.HookTypeStatic,
		Parameters: map[string]string{
			"message": "Static response message",
		},
		CreatedAt: time.Now(),
	}

	// Save both hooks
	err := s.SaveRegisteredHook(hook1)
	if err != nil {
		t.Fatalf("SaveRegisteredHook hook1 failed: %v", err)
	}

	err = s.SaveRegisteredHook(hook2)
	if err != nil {
		t.Fatalf("SaveRegisteredHook hook2 failed: %v", err)
	}

	// List all hooks
	hooks, err := s.ListRegisteredHooks()
	if err != nil {
		t.Fatalf("ListRegisteredHooks failed: %v", err)
	}

	if len(hooks) != 2 {
		t.Errorf("Expected 2 hooks, got %d", len(hooks))
	}

	// Verify both hooks exist
	retrievedHook1, err := s.GetRegisteredHook("+1111111111")
	if err != nil {
		t.Fatalf("GetRegisteredHook hook1 failed: %v", err)
	}
	if retrievedHook1 == nil {
		t.Fatal("Hook1 should exist")
	}

	retrievedHook2, err := s.GetRegisteredHook("+2222222222")
	if err != nil {
		t.Fatalf("GetRegisteredHook hook2 failed: %v", err)
	}
	if retrievedHook2 == nil {
		t.Fatal("Hook2 should exist")
	}

	// Delete one hook
	err = s.DeleteRegisteredHook("+1111111111")
	if err != nil {
		t.Fatalf("DeleteRegisteredHook hook1 failed: %v", err)
	}

	// Verify only one hook remains
	hooks, err = s.ListRegisteredHooks()
	if err != nil {
		t.Fatalf("ListRegisteredHooks after deletion failed: %v", err)
	}

	if len(hooks) != 1 {
		t.Errorf("Expected 1 hook after deletion, got %d", len(hooks))
	}

	if hooks[0].PhoneNumber != "+2222222222" {
		t.Errorf("Wrong hook remained: expected '+2222222222', got '%s'", hooks[0].PhoneNumber)
	}
}

func TestInMemoryStore_HookPersistence_UpdateHook(t *testing.T) {
	s := NewInMemoryStore()

	// Save initial hook
	hook := models.RegisteredHook{
		PhoneNumber: "+1234567890",
		HookType:    models.HookTypeConversation,
		Parameters: map[string]string{
			"participant_id": "test-participant",
			"phone_number":   "+1234567890",
		},
		CreatedAt: time.Now(),
	}

	err := s.SaveRegisteredHook(hook)
	if err != nil {
		t.Fatalf("SaveRegisteredHook failed: %v", err)
	}

	// Update the hook (same phone number, different parameters)
	updatedHook := models.RegisteredHook{
		PhoneNumber: "+1234567890",
		HookType:    models.HookTypeStatic,
		Parameters: map[string]string{
			"message": "Updated static message",
		},
		CreatedAt: time.Now().Add(time.Hour),
	}

	err = s.SaveRegisteredHook(updatedHook)
	if err != nil {
		t.Fatalf("SaveRegisteredHook update failed: %v", err)
	}

	// Verify hook was updated
	retrievedHook, err := s.GetRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("GetRegisteredHook after update failed: %v", err)
	}

	if retrievedHook == nil {
		t.Fatal("Updated hook is nil")
	}

	if retrievedHook.HookType != models.HookTypeStatic {
		t.Errorf("Expected hook type '%s', got '%s'", models.HookTypeStatic, retrievedHook.HookType)
	}

	if retrievedHook.Parameters["message"] != "Updated static message" {
		t.Errorf("Expected message 'Updated static message', got '%s'",
			retrievedHook.Parameters["message"])
	}

	// Verify still only one hook exists
	hooks, err := s.ListRegisteredHooks()
	if err != nil {
		t.Fatalf("ListRegisteredHooks after update failed: %v", err)
	}

	if len(hooks) != 1 {
		t.Errorf("Expected 1 hook after update, got %d", len(hooks))
	}
}

func TestInMemoryStore_HookPersistence_NonExistentHook(t *testing.T) {
	s := NewInMemoryStore()

	// Try to get a non-existent hook
	hook, err := s.GetRegisteredHook("+9999999999")
	if err != nil {
		t.Fatalf("GetRegisteredHook for non-existent hook failed: %v", err)
	}

	if hook != nil {
		t.Error("Expected nil for non-existent hook")
	}

	// Try to delete a non-existent hook (should not error)
	err = s.DeleteRegisteredHook("+9999999999")
	if err != nil {
		t.Fatalf("DeleteRegisteredHook for non-existent hook failed: %v", err)
	}

	// List should be empty
	hooks, err := s.ListRegisteredHooks()
	if err != nil {
		t.Fatalf("ListRegisteredHooks for empty store failed: %v", err)
	}

	if len(hooks) != 0 {
		t.Errorf("Expected 0 hooks for empty store, got %d", len(hooks))
	}
}

func TestSQLiteStore_HookPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlite_hook_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "hook_test.db")
	s, err := NewSQLiteStore(WithSQLiteDSN(dbPath))
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer s.Close()

	// Test saving a hook
	hook := models.RegisteredHook{
		PhoneNumber: "+1234567890",
		HookType:    models.HookTypeConversation,
		Parameters: map[string]string{
			"participant_id": "test-participant",
			"phone_number":   "+1234567890",
		},
		CreatedAt: time.Now(),
	}

	err = s.SaveRegisteredHook(hook)
	if err != nil {
		t.Fatalf("SaveRegisteredHook failed: %v", err)
	}

	// Test getting the hook
	retrievedHook, err := s.GetRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("GetRegisteredHook failed: %v", err)
	}

	if retrievedHook == nil {
		t.Fatal("Retrieved hook is nil")
	}

	if retrievedHook.PhoneNumber != hook.PhoneNumber {
		t.Errorf("Expected phone number '%s', got '%s'", hook.PhoneNumber, retrievedHook.PhoneNumber)
	}

	if retrievedHook.HookType != hook.HookType {
		t.Errorf("Expected hook type '%s', got '%s'", hook.HookType, retrievedHook.HookType)
	}

	if retrievedHook.Parameters["participant_id"] != hook.Parameters["participant_id"] {
		t.Errorf("Expected participant_id '%s', got '%s'",
			hook.Parameters["participant_id"], retrievedHook.Parameters["participant_id"])
	}

	// Test listing hooks
	hooks, err := s.ListRegisteredHooks()
	if err != nil {
		t.Fatalf("ListRegisteredHooks failed: %v", err)
	}

	if len(hooks) != 1 {
		t.Errorf("Expected 1 hook, got %d", len(hooks))
	}

	// Test deleting the hook
	err = s.DeleteRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("DeleteRegisteredHook failed: %v", err)
	}

	// Verify hook is deleted
	deletedHook, err := s.GetRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("GetRegisteredHook after deletion failed: %v", err)
	}

	if deletedHook != nil {
		t.Error("Hook should have been deleted")
	}
}

func TestPostgresStore_HookPersistence(t *testing.T) {
	dsn := getenvOrSkip(t, "POSTGRES_DSN_TEST")
	if dsn == "" {
		t.Skip("POSTGRES_DSN_TEST not set, skipping PostgresStore hook persistence tests")
	}

	s, err := NewPostgresStore(WithPostgresDSN(dsn))
	if err != nil {
		t.Fatalf("NewPostgresStore failed: %v", err)
	}
	defer s.Close()

	// Clean up any existing hooks first
	hooks, err := s.ListRegisteredHooks()
	if err != nil {
		t.Fatalf("ListRegisteredHooks cleanup failed: %v", err)
	}
	for _, hook := range hooks {
		s.DeleteRegisteredHook(hook.PhoneNumber)
	}

	// Test saving a hook
	hook := models.RegisteredHook{
		PhoneNumber: "+1234567890",
		HookType:    models.HookTypeConversation,
		Parameters: map[string]string{
			"participant_id": "test-participant",
			"phone_number":   "+1234567890",
		},
		CreatedAt: time.Now(),
	}

	err = s.SaveRegisteredHook(hook)
	if err != nil {
		t.Fatalf("SaveRegisteredHook failed: %v", err)
	}

	// Test getting the hook
	retrievedHook, err := s.GetRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("GetRegisteredHook failed: %v", err)
	}

	if retrievedHook == nil {
		t.Fatal("Retrieved hook is nil")
	}

	if retrievedHook.PhoneNumber != hook.PhoneNumber {
		t.Errorf("Expected phone number '%s', got '%s'", hook.PhoneNumber, retrievedHook.PhoneNumber)
	}

	if retrievedHook.HookType != hook.HookType {
		t.Errorf("Expected hook type '%s', got '%s'", hook.HookType, retrievedHook.HookType)
	}

	if retrievedHook.Parameters["participant_id"] != hook.Parameters["participant_id"] {
		t.Errorf("Expected participant_id '%s', got '%s'",
			hook.Parameters["participant_id"], retrievedHook.Parameters["participant_id"])
	}

	// Test listing hooks
	hooks, err = s.ListRegisteredHooks()
	if err != nil {
		t.Fatalf("ListRegisteredHooks failed: %v", err)
	}

	found := false
	for _, h := range hooks {
		if h.PhoneNumber == "+1234567890" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Hook not found in list")
	}

	// Test deleting the hook
	err = s.DeleteRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("DeleteRegisteredHook failed: %v", err)
	}

	// Verify hook is deleted
	deletedHook, err := s.GetRegisteredHook("+1234567890")
	if err != nil {
		t.Fatalf("GetRegisteredHook after deletion failed: %v", err)
	}

	if deletedHook != nil {
		t.Error("Hook should have been deleted")
	}
}

func getenvOrSkip(t *testing.T, key string) string {
	t.Helper()
	val := os.Getenv(key)
	if val == "" {
		// t.Skipf("Environment variable %s not set, skipping test", key)
	}
	return val
}
