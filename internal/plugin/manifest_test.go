package plugin

import (
	"encoding/json"
	"os"
	"testing"
)

func TestManifestParsing(t *testing.T) {
	plugins := []string{"webshell", "filemanager"}

	for _, name := range plugins {
		path := "../../plugins/" + name + "/plugin.json"
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("Failed to read %s: %v", path, err)
			continue
		}

		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			t.Errorf("Failed to parse %s: %v", name, err)
			continue
		}

		if err := m.Validate(); err != nil {
			t.Errorf("Validation failed for %s: %v", name, err)
			continue
		}

		t.Logf("✓ %s v%s (%s) - Image: %s", m.Name, m.Version, m.Type, m.Container.Image)

		// Verify privileged plugins
		if m.IsPrivileged() {
			if !m.RequiresApproval() {
				t.Errorf("%s is privileged but doesn't require approval", name)
			}
		}

		// Verify container config
		if m.Container.Image == "" {
			t.Errorf("%s has no container image", name)
		}
		if m.Container.Port == 0 {
			t.Errorf("%s has no container port", name)
		}
	}
}

func TestManifestValidation(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name: "minimal valid",
			json: `{
				"name": "test",
				"version": "1.0.0",
				"type": "normal",
				"container": {
					"image": "test:latest",
					"port": 8080
				}
			}`,
			wantErr: false,
		},
		{
			name: "missing name",
			json: `{
				"version": "1.0.0",
				"type": "normal",
				"container": {"image": "test:latest", "port": 8080}
			}`,
			wantErr: true,
		},
		{
			name: "missing image",
			json: `{
				"name": "test",
				"version": "1.0.0",
				"type": "normal",
				"container": {"port": 8080}
			}`,
			wantErr: true,
		},
		{
			name: "invalid type",
			json: `{
				"name": "test",
				"version": "1.0.0",
				"type": "invalid",
				"container": {"image": "test:latest", "port": 8080}
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m Manifest
			if err := json.Unmarshal([]byte(tt.json), &m); err != nil {
				t.Fatalf("JSON parse failed: %v", err)
			}
			err := m.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBaseURL(t *testing.T) {
	m := &Manifest{
		Name: "test",
		Container: ContainerConfig{
			Port:     8080,
			HostPort: 38100,
		},
	}

	// With explicit host port
	url := m.BaseURL(38150)
	if url != "http://127.0.0.1:38150" {
		t.Errorf("Expected http://127.0.0.1:38150, got %s", url)
	}

	// With zero - use container's hostPort
	url = m.BaseURL(0)
	if url != "http://127.0.0.1:38100" {
		t.Errorf("Expected http://127.0.0.1:38100, got %s", url)
	}
}

func TestContainerNameOrDefault(t *testing.T) {
	m := &Manifest{Name: "mytest"}

	// Default name
	if n := m.ContainerNameOrDefault(); n != "web-monitor-plugin-mytest" {
		t.Errorf("Expected web-monitor-plugin-mytest, got %s", n)
	}

	// Custom name
	m.Container.ContainerName = "custom-name"
	if n := m.ContainerNameOrDefault(); n != "custom-name" {
		t.Errorf("Expected custom-name, got %s", n)
	}
}
