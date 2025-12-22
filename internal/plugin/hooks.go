package plugin

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// HookExecutor executes installation hooks with rollback support
type HookExecutor struct {
	manifest *Manifest
	executed []executedHook // Stack of executed hooks for rollback
	dryRun   bool           // If true, only validate without executing
}

type executedHook struct {
	hook     InstallHook
	rollback func() error
}

// NewHookExecutor creates a new hook executor
func NewHookExecutor(manifest *Manifest) *HookExecutor {
	return &HookExecutor{
		manifest: manifest,
		executed: make([]executedHook, 0),
	}
}

// ExecuteInstallHooks runs all install hooks with rollback on failure
func (e *HookExecutor) ExecuteInstallHooks() error {
	if e.manifest.Install == nil || len(e.manifest.Install.Hooks) == 0 {
		return nil
	}

	for i, hook := range e.manifest.Install.Hooks {
		if err := e.executeHook(hook); err != nil {
			// Rollback all previously executed hooks
			rollbackErr := e.Rollback()
			if rollbackErr != nil {
				return fmt.Errorf("hook %d (%s) failed: %v; rollback also failed: %v",
					i, hook.Type, err, rollbackErr)
			}
			return fmt.Errorf("hook %d (%s) failed: %v (rolled back)", i, hook.Type, err)
		}
	}

	return nil
}

// ExecuteUninstallHooks runs all uninstall hooks
func (e *HookExecutor) ExecuteUninstallHooks() error {
	if e.manifest.Uninstall == nil || len(e.manifest.Uninstall.Hooks) == 0 {
		return nil
	}

	var errs []string
	for _, hook := range e.manifest.Uninstall.Hooks {
		if err := e.executeHook(hook); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", hook.Type, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("uninstall errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Rollback undoes all executed hooks in reverse order
func (e *HookExecutor) Rollback() error {
	var errs []string

	// Rollback in reverse order
	for i := len(e.executed) - 1; i >= 0; i-- {
		eh := e.executed[i]
		if eh.rollback != nil {
			if err := eh.rollback(); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", eh.hook.Type, err))
			}
		}
	}

	e.executed = nil

	if len(errs) > 0 {
		return fmt.Errorf("rollback errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// executeHook dispatches to the appropriate hook handler
func (e *HookExecutor) executeHook(hook InstallHook) error {
	var err error
	var rollback func() error

	switch hook.Type {
	case HookEnsureUser:
		rollback, err = e.executeEnsureUser(hook)
	case HookGenerateSSHKey:
		rollback, err = e.executeGenerateSSHKey(hook)
	case HookAuthorizeKey:
		rollback, err = e.executeAuthorizeKey(hook)
	case HookRemoveAuthorizedKey:
		err = e.executeRemoveAuthorizedKey(hook)
	case HookCreateDirectory:
		rollback, err = e.executeCreateDirectory(hook)
	case HookWriteConfig:
		rollback, err = e.executeWriteConfig(hook)
	case HookRemoveFile:
		err = e.executeRemoveFile(hook)
	default:
		return fmt.Errorf("unknown hook type: %s", hook.Type)
	}

	if err != nil {
		return err
	}

	e.executed = append(e.executed, executedHook{hook: hook, rollback: rollback})
	return nil
}

// ============================================================================
// Hook Implementations
// ============================================================================

// executeEnsureUser creates a system user if it doesn't exist
func (e *HookExecutor) executeEnsureUser(hook InstallHook) (func() error, error) {
	username := hook.User
	if username == "" {
		return nil, fmt.Errorf("ensure-user: user is required")
	}

	// Check if user already exists
	_, err := user.Lookup(username)
	if err == nil {
		// User exists, no action needed
		fmt.Printf("  [ensure-user] User %s already exists\n", username)
		return nil, nil
	}

	// Create user
	shell := hook.Shell
	if shell == "" {
		shell = "/bin/bash"
	}

	args := []string{
		"--system",
		"--shell", shell,
		"--home", "/home/" + username,
		"--create-home",
		username,
	}

	if e.dryRun {
		fmt.Printf("  [ensure-user] Would run: useradd %s\n", strings.Join(args, " "))
		return nil, nil
	}

	cmd := exec.Command("useradd", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("useradd failed: %v: %s", err, output)
	}

	fmt.Printf("  [ensure-user] Created user %s\n", username)

	// Rollback: delete the user
	rollback := func() error {
		cmd := exec.Command("userdel", "-r", username)
		_, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("userdel failed: %v", err)
		}
		fmt.Printf("  [rollback] Deleted user %s\n", username)
		return nil
	}

	return rollback, nil
}

// executeGenerateSSHKey generates an SSH key pair
func (e *HookExecutor) executeGenerateSSHKey(hook InstallHook) (func() error, error) {
	keyPath := hook.KeyPath
	if keyPath == "" {
		return nil, fmt.Errorf("generate-ssh-key: keyPath is required")
	}

	algorithm := hook.Algorithm
	if algorithm == "" {
		algorithm = "ed25519"
	}

	// Validate algorithm
	validAlgorithms := map[string]bool{"ed25519": true, "rsa": true, "ecdsa": true}
	if !validAlgorithms[algorithm] {
		return nil, fmt.Errorf("generate-ssh-key: invalid algorithm %s", algorithm)
	}

	// Check if key already exists
	if _, err := os.Stat(keyPath); err == nil {
		fmt.Printf("  [generate-ssh-key] Key %s already exists\n", keyPath)
		return nil, nil
	}

	// Ensure directory exists
	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	if e.dryRun {
		fmt.Printf("  [generate-ssh-key] Would generate %s key at %s\n", algorithm, keyPath)
		return nil, nil
	}

	// Generate key
	args := []string{
		"-t", algorithm,
		"-f", keyPath,
		"-N", "", // Empty passphrase
		"-C", fmt.Sprintf("opskernel-plugin-%s", e.manifest.Name),
	}

	cmd := exec.Command("ssh-keygen", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ssh-keygen failed: %v: %s", err, output)
	}

	// Set proper permissions
	if err := os.Chmod(keyPath, 0600); err != nil {
		return nil, fmt.Errorf("failed to set key permissions: %v", err)
	}

	fmt.Printf("  [generate-ssh-key] Generated %s key at %s\n", algorithm, keyPath)

	// Rollback: remove the key files
	rollback := func() error {
		os.Remove(keyPath)
		os.Remove(keyPath + ".pub")
		fmt.Printf("  [rollback] Removed SSH key %s\n", keyPath)
		return nil
	}

	return rollback, nil
}

// executeAuthorizeKey adds a public key to a user's authorized_keys
func (e *HookExecutor) executeAuthorizeKey(hook InstallHook) (func() error, error) {
	username := hook.User
	keyPath := hook.KeyPath
	if username == "" || keyPath == "" {
		return nil, fmt.Errorf("authorize-key: user and keyPath are required")
	}

	// Read public key
	pubKey, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key %s: %v", keyPath, err)
	}
	pubKeyStr := strings.TrimSpace(string(pubKey))

	// Get user info
	u, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("user %s not found: %v", username, err)
	}

	// Paths
	sshDir := filepath.Join(u.HomeDir, ".ssh")
	authKeysPath := filepath.Join(sshDir, "authorized_keys")

	// Check if key is already authorized
	if existingKeys, err := os.ReadFile(authKeysPath); err == nil {
		if strings.Contains(string(existingKeys), pubKeyStr) {
			fmt.Printf("  [authorize-key] Key already in %s\n", authKeysPath)
			return nil, nil
		}
	}

	if e.dryRun {
		fmt.Printf("  [authorize-key] Would add key to %s\n", authKeysPath)
		return nil, nil
	}

	// Create .ssh directory if needed
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create .ssh directory: %v", err)
	}

	// Set ownership of .ssh directory
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	if err := os.Chown(sshDir, uid, gid); err != nil {
		return nil, fmt.Errorf("failed to chown .ssh: %v", err)
	}

	// Append to authorized_keys
	f, err := os.OpenFile(authKeysPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open authorized_keys: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(pubKeyStr + "\n"); err != nil {
		return nil, fmt.Errorf("failed to write authorized_keys: %v", err)
	}

	// Set ownership
	if err := os.Chown(authKeysPath, uid, gid); err != nil {
		return nil, fmt.Errorf("failed to chown authorized_keys: %v", err)
	}

	fmt.Printf("  [authorize-key] Added key to %s\n", authKeysPath)

	// Rollback: remove the key from authorized_keys
	rollback := func() error {
		return removeKeyFromAuthorizedKeys(authKeysPath, pubKeyStr)
	}

	return rollback, nil
}

// executeRemoveAuthorizedKey removes a public key from authorized_keys
func (e *HookExecutor) executeRemoveAuthorizedKey(hook InstallHook) error {
	username := hook.User
	keyPath := hook.KeyPath
	if username == "" || keyPath == "" {
		return fmt.Errorf("remove-authorized-key: user and keyPath are required")
	}

	// Read public key
	pubKey, err := os.ReadFile(keyPath)
	if err != nil {
		// Key file doesn't exist, nothing to remove
		fmt.Printf("  [remove-authorized-key] Key file %s not found, skipping\n", keyPath)
		return nil
	}
	pubKeyStr := strings.TrimSpace(string(pubKey))

	// Get user info
	u, err := user.Lookup(username)
	if err != nil {
		// User doesn't exist, nothing to do
		fmt.Printf("  [remove-authorized-key] User %s not found, skipping\n", username)
		return nil
	}

	authKeysPath := filepath.Join(u.HomeDir, ".ssh", "authorized_keys")

	if e.dryRun {
		fmt.Printf("  [remove-authorized-key] Would remove key from %s\n", authKeysPath)
		return nil
	}

	return removeKeyFromAuthorizedKeys(authKeysPath, pubKeyStr)
}

// executeCreateDirectory creates a directory with specified permissions
func (e *HookExecutor) executeCreateDirectory(hook InstallHook) (func() error, error) {
	path := hook.Path
	if path == "" {
		return nil, fmt.Errorf("create-directory: path is required")
	}

	// Check if directory already exists
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		fmt.Printf("  [create-directory] Directory %s already exists\n", path)
		return nil, nil
	}

	if e.dryRun {
		fmt.Printf("  [create-directory] Would create %s\n", path)
		return nil, nil
	}

	// Parse mode
	mode := os.FileMode(0755)
	if hook.Mode != "" {
		parsed, err := strconv.ParseUint(hook.Mode, 8, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid mode %s: %v", hook.Mode, err)
		}
		mode = os.FileMode(parsed)
	}

	if err := os.MkdirAll(path, mode); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}

	fmt.Printf("  [create-directory] Created %s\n", path)

	// Rollback: remove directory (only if empty)
	rollback := func() error {
		if err := os.Remove(path); err != nil {
			// Don't fail rollback if directory not empty
			fmt.Printf("  [rollback] Could not remove %s (may not be empty)\n", path)
			return nil
		}
		fmt.Printf("  [rollback] Removed directory %s\n", path)
		return nil
	}

	return rollback, nil
}

// executeWriteConfig writes a configuration file
func (e *HookExecutor) executeWriteConfig(hook InstallHook) (func() error, error) {
	path := hook.Path
	content := hook.Content
	if path == "" {
		return nil, fmt.Errorf("write-config: path is required")
	}

	// Check if file already exists
	var existingContent []byte
	if data, err := os.ReadFile(path); err == nil {
		existingContent = data
	}

	if e.dryRun {
		fmt.Printf("  [write-config] Would write to %s\n", path)
		return nil, nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}

	// Parse mode
	mode := os.FileMode(0644)
	if hook.Mode != "" {
		parsed, err := strconv.ParseUint(hook.Mode, 8, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid mode %s: %v", hook.Mode, err)
		}
		mode = os.FileMode(parsed)
	}

	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return nil, fmt.Errorf("failed to write file: %v", err)
	}

	fmt.Printf("  [write-config] Wrote %s\n", path)

	// Rollback: restore previous content or delete
	rollback := func() error {
		if existingContent != nil {
			os.WriteFile(path, existingContent, mode)
			fmt.Printf("  [rollback] Restored %s\n", path)
		} else {
			os.Remove(path)
			fmt.Printf("  [rollback] Removed %s\n", path)
		}
		return nil
	}

	return rollback, nil
}

// executeRemoveFile removes a file
func (e *HookExecutor) executeRemoveFile(hook InstallHook) error {
	path := hook.Path
	if path == "" {
		return fmt.Errorf("remove-file: path is required")
	}

	if e.dryRun {
		fmt.Printf("  [remove-file] Would remove %s\n", path)
		return nil
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove file: %v", err)
	}

	fmt.Printf("  [remove-file] Removed %s\n", path)
	return nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// removeKeyFromAuthorizedKeys removes a specific key from authorized_keys
func removeKeyFromAuthorizedKeys(authKeysPath, pubKeyStr string) error {
	data, err := os.ReadFile(authKeysPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var newLines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != pubKeyStr {
			newLines = append(newLines, line)
		}
	}

	newContent := strings.Join(newLines, "\n")
	if len(newLines) > 0 {
		newContent += "\n"
	}

	if err := os.WriteFile(authKeysPath, []byte(newContent), 0600); err != nil {
		return err
	}

	fmt.Printf("  [remove-authorized-key] Removed key from %s\n", authKeysPath)
	return nil
}

// ValidateHooks checks if all hooks in a manifest are valid before execution
func ValidateHooks(manifest *Manifest) error {
	if manifest.Install == nil {
		return nil
	}

	for i, hook := range manifest.Install.Hooks {
		if err := validateHook(hook); err != nil {
			return fmt.Errorf("install hook %d: %v", i, err)
		}
	}

	if manifest.Uninstall != nil {
		for i, hook := range manifest.Uninstall.Hooks {
			if err := validateHook(hook); err != nil {
				return fmt.Errorf("uninstall hook %d: %v", i, err)
			}
		}
	}

	return nil
}

func validateHook(hook InstallHook) error {
	switch hook.Type {
	case HookEnsureUser:
		if hook.User == "" {
			return fmt.Errorf("ensure-user requires user")
		}
	case HookGenerateSSHKey:
		if hook.KeyPath == "" {
			return fmt.Errorf("generate-ssh-key requires keyPath")
		}
	case HookAuthorizeKey, HookRemoveAuthorizedKey:
		if hook.User == "" || hook.KeyPath == "" {
			return fmt.Errorf("%s requires user and keyPath", hook.Type)
		}
	case HookCreateDirectory, HookRemoveFile:
		if hook.Path == "" {
			return fmt.Errorf("%s requires path", hook.Type)
		}
	case HookWriteConfig:
		if hook.Path == "" {
			return fmt.Errorf("write-config requires path")
		}
	default:
		return fmt.Errorf("unknown hook type: %s", hook.Type)
	}
	return nil
}
