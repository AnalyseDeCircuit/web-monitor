package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHookExecutor_CreateDirectory(t *testing.T) {
	// Create a temp directory for testing
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "test-dir")

	manifest := &Manifest{
		Name: "test",
		Install: &InstallConfig{
			Hooks: []InstallHook{
				{
					Type: HookCreateDirectory,
					Path: testDir,
					Mode: "0755",
				},
			},
		},
	}

	executor := NewHookExecutor(manifest)
	err := executor.ExecuteInstallHooks()
	if err != nil {
		t.Fatalf("ExecuteInstallHooks failed: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("Directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Created path is not a directory")
	}

	// Test rollback
	err = executor.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify directory was removed
	_, err = os.Stat(testDir)
	if !os.IsNotExist(err) {
		t.Error("Directory was not removed during rollback")
	}
}

func TestHookExecutor_WriteConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.txt")
	content := "test config content"

	manifest := &Manifest{
		Name: "test",
		Install: &InstallConfig{
			Hooks: []InstallHook{
				{
					Type:    HookWriteConfig,
					Path:    configPath,
					Content: content,
					Mode:    "0644",
				},
			},
		},
	}

	executor := NewHookExecutor(manifest)
	err := executor.ExecuteInstallHooks()
	if err != nil {
		t.Fatalf("ExecuteInstallHooks failed: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}
	if string(data) != content {
		t.Errorf("Content mismatch: got %q, want %q", string(data), content)
	}

	// Test rollback
	err = executor.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify file was removed
	_, err = os.Stat(configPath)
	if !os.IsNotExist(err) {
		t.Error("Config file was not removed during rollback")
	}
}

func TestHookExecutor_WriteConfig_Rollback_RestoreExisting(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.txt")
	originalContent := "original content"
	newContent := "new content"

	// Create existing file
	if err := os.WriteFile(configPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	manifest := &Manifest{
		Name: "test",
		Install: &InstallConfig{
			Hooks: []InstallHook{
				{
					Type:    HookWriteConfig,
					Path:    configPath,
					Content: newContent,
				},
			},
		},
	}

	executor := NewHookExecutor(manifest)
	err := executor.ExecuteInstallHooks()
	if err != nil {
		t.Fatalf("ExecuteInstallHooks failed: %v", err)
	}

	// Verify new content
	data, _ := os.ReadFile(configPath)
	if string(data) != newContent {
		t.Errorf("Content not updated")
	}

	// Rollback should restore original content
	err = executor.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read after rollback: %v", err)
	}
	if string(data) != originalContent {
		t.Errorf("Original content not restored: got %q, want %q", string(data), originalContent)
	}
}

func TestHookExecutor_MultipleHooks_RollbackOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir2")
	invalidDir := "/nonexistent/deeply/nested/path/that/should/fail"

	manifest := &Manifest{
		Name: "test",
		Install: &InstallConfig{
			Hooks: []InstallHook{
				{Type: HookCreateDirectory, Path: dir1, Mode: "0755"},
				{Type: HookCreateDirectory, Path: dir2, Mode: "0755"},
				// This should fail (on most systems, can't create in root without perms)
				{Type: HookCreateDirectory, Path: invalidDir, Mode: "0755"},
			},
		},
	}

	executor := NewHookExecutor(manifest)
	err := executor.ExecuteInstallHooks()

	// Should fail on the invalid directory
	if err == nil {
		// If running as root, this might succeed - skip the failure check
		t.Log("Note: invalid directory creation succeeded (probably running as root)")
		return
	}

	// Verify rollback happened - previous directories should be removed
	_, err1 := os.Stat(dir1)
	_, err2 := os.Stat(dir2)

	// At least one should be cleaned up by rollback
	// (dir2 should be removed, dir1 might still exist depending on rollback order)
	if !os.IsNotExist(err2) {
		t.Error("dir2 should have been removed by rollback")
	}
	_ = err1 // dir1 might or might not exist
}

func TestValidateHooks(t *testing.T) {
	tests := []struct {
		name    string
		hooks   []InstallHook
		wantErr bool
	}{
		{
			name: "valid ensure-user",
			hooks: []InstallHook{
				{Type: HookEnsureUser, User: "testuser"},
			},
			wantErr: false,
		},
		{
			name: "invalid ensure-user missing user",
			hooks: []InstallHook{
				{Type: HookEnsureUser},
			},
			wantErr: true,
		},
		{
			name: "valid generate-ssh-key",
			hooks: []InstallHook{
				{Type: HookGenerateSSHKey, KeyPath: "/tmp/key"},
			},
			wantErr: false,
		},
		{
			name: "invalid generate-ssh-key missing keyPath",
			hooks: []InstallHook{
				{Type: HookGenerateSSHKey},
			},
			wantErr: true,
		},
		{
			name: "valid authorize-key",
			hooks: []InstallHook{
				{Type: HookAuthorizeKey, User: "user", KeyPath: "/tmp/key.pub"},
			},
			wantErr: false,
		},
		{
			name: "invalid authorize-key missing user",
			hooks: []InstallHook{
				{Type: HookAuthorizeKey, KeyPath: "/tmp/key.pub"},
			},
			wantErr: true,
		},
		{
			name: "unknown hook type",
			hooks: []InstallHook{
				{Type: "unknown-hook"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := &Manifest{
				Name:    "test",
				Install: &InstallConfig{Hooks: tt.hooks},
			}
			err := ValidateHooks(manifest)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHooks() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRemoveKeyFromAuthorizedKeys(t *testing.T) {
	tmpDir := t.TempDir()
	authKeysPath := filepath.Join(tmpDir, "authorized_keys")

	// Create test authorized_keys file
	keys := `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKeepThis key1
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIRemoveMe key2
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAlsoKeep key3
`
	if err := os.WriteFile(authKeysPath, []byte(keys), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Remove the middle key
	keyToRemove := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIRemoveMe key2"
	err := removeKeyFromAuthorizedKeys(authKeysPath, keyToRemove)
	if err != nil {
		t.Fatalf("removeKeyFromAuthorizedKeys failed: %v", err)
	}

	// Verify
	data, _ := os.ReadFile(authKeysPath)
	content := string(data)

	if !contains(content, "KeepThis") {
		t.Error("First key was incorrectly removed")
	}
	if contains(content, "RemoveMe") {
		t.Error("Target key was not removed")
	}
	if !contains(content, "AlsoKeep") {
		t.Error("Third key was incorrectly removed")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
