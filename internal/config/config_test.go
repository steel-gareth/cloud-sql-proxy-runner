package config

import (
	"strings"
	"testing"
)

func TestValidConfig(t *testing.T) {
	yaml := `
proxies:
  - instance: "org-123456:us-central1:org-clone"
    port: 5432
    secret: "app-db-user-password"
  - instance: "org-staging:us-central1:org"
    port: 5433
    secret: "app-db-user-password"
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(cfg.Proxies))
	}
	if cfg.Proxies[0].Instance != "org-123456:us-central1:org-clone" {
		t.Errorf("unexpected instance: %s", cfg.Proxies[0].Instance)
	}
	if cfg.Proxies[0].Port != 5432 {
		t.Errorf("unexpected port: %d", cfg.Proxies[0].Port)
	}
	if cfg.Proxies[0].Secret != "app-db-user-password" {
		t.Errorf("unexpected secret: %s", cfg.Proxies[0].Secret)
	}
}

func TestMissingProxiesKey(t *testing.T) {
	yaml := `foo: bar`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Invalid config") {
		t.Errorf("expected 'Invalid config' in error, got: %v", err)
	}
}

func TestEmptyProxiesArray(t *testing.T) {
	yaml := `proxies: []`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Invalid config") {
		t.Errorf("expected 'Invalid config' in error, got: %v", err)
	}
}

func TestMissingRequiredField(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "missing instance",
			yaml: `proxies:
  - port: 5432
    secret: "pw"`,
			want: "instance",
		},
		{
			name: "missing port",
			yaml: `proxies:
  - instance: "proj:region:name"
    secret: "pw"`,
			want: "port",
		},
		{
			name: "missing secret",
			yaml: `proxies:
  - instance: "proj:region:name"
    port: 5432`,
			want: "secret",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected error to mention %q, got: %v", tt.want, err)
			}
		})
	}
}

func TestInvalidInstanceFormat(t *testing.T) {
	tests := []string{
		`proxies:
  - instance: "bad"
    port: 5432
    secret: "pw"`,
		`proxies:
  - instance: "a:b"
    port: 5432
    secret: "pw"`,
	}
	for _, yaml := range tests {
		_, err := Parse([]byte(yaml))
		if err == nil {
			t.Fatalf("expected error for yaml: %s", yaml)
		}
		if !strings.Contains(err.Error(), "Invalid config") {
			t.Errorf("expected 'Invalid config' in error, got: %v", err)
		}
	}
}

func TestPortOutOfRange(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "port 0",
			yaml: `proxies:
  - instance: "proj:region:name"
    port: 0
    secret: "pw"`,
		},
		{
			name: "port 1023",
			yaml: `proxies:
  - instance: "proj:region:name"
    port: 1023
    secret: "pw"`,
		},
		{
			name: "port 65536",
			yaml: `proxies:
  - instance: "proj:region:name"
    port: 65536
    secret: "pw"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "Invalid config") {
				t.Errorf("expected 'Invalid config' in error, got: %v", err)
			}
		})
	}
}

func TestEmptySecret(t *testing.T) {
	yaml := `proxies:
  - instance: "proj:region:name"
    port: 5432
    secret: ""`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Invalid config") {
		t.Errorf("expected 'Invalid config' in error, got: %v", err)
	}
}

func TestDuplicatePorts(t *testing.T) {
	yaml := `proxies:
  - instance: "proj1:region:name1"
    port: 5432
    secret: "pw1"
  - instance: "proj2:region:name2"
    port: 5432
    secret: "pw2"`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate port") {
		t.Errorf("expected 'duplicate port' in error, got: %v", err)
	}
}

func TestDuplicateInstances(t *testing.T) {
	yaml := `proxies:
  - instance: "proj:region:name"
    port: 5432
    secret: "pw1"
  - instance: "proj:region:name"
    port: 5433
    secret: "pw2"`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate instance") {
		t.Errorf("expected 'duplicate instance' in error, got: %v", err)
	}
}

func TestExtraUnknownField(t *testing.T) {
	yaml := `proxies:
  - instance: "proj:region:name"
    port: 5432
    secret: "pw"
    extra: "bad"`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for extra field")
	}
	if !strings.Contains(err.Error(), "Invalid config") {
		t.Errorf("expected 'Invalid config' in error, got: %v", err)
	}
}

func TestExtraTopLevelField(t *testing.T) {
	yaml := `proxies:
  - instance: "proj:region:name"
    port: 5432
    secret: "pw"
extra: "bad"`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for extra top-level field")
	}
}

func TestProjectParsedFromInstance(t *testing.T) {
	yaml := `proxies:
  - instance: "org-123456:us-central1:org-clone"
    port: 5432
    secret: "pw"`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	project := cfg.Proxies[0].Project()
	if project != "org-123456" {
		t.Errorf("expected project 'org-123456', got %q", project)
	}
}
