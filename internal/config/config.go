package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"
)

var printer = message.NewPrinter(language.English)

//go:embed schema.json
var schemaJSON []byte

type ProxyEntry struct {
	Instance string `yaml:"instance" json:"instance"`
	Port     int    `yaml:"port" json:"port"`
	Secret   string `yaml:"secret" json:"secret"`
}

func (p ProxyEntry) Project() string {
	parts := strings.SplitN(p.Instance, ":", 2)
	if len(parts) < 1 {
		return ""
	}
	return parts[0]
}

type Config struct {
	Proxies []ProxyEntry `yaml:"proxies" json:"proxies"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	// Parse YAML into a generic interface for schema validation
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	// Validate against JSON Schema
	if err := validateSchema(raw); err != nil {
		return nil, err
	}

	// Parse into typed struct
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Go-level uniqueness checks
	if err := validateUniqueness(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validateSchema(data any) error {
	var schemaDoc any
	if err := json.Unmarshal(schemaJSON, &schemaDoc); err != nil {
		return fmt.Errorf("parsing schema: %w", err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", schemaDoc); err != nil {
		return fmt.Errorf("adding schema resource: %w", err)
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("compiling schema: %w", err)
	}

	if err := sch.Validate(data); err != nil {
		return fmt.Errorf("Invalid config: %s", formatValidationError(err))
	}
	return nil
}

func formatValidationError(err error) string {
	if ve, ok := err.(*jsonschema.ValidationError); ok {
		if len(ve.Causes) > 0 {
			return formatValidationError(ve.Causes[0])
		}
		path := strings.Join(ve.InstanceLocation, ".")
		if path == "" {
			path = "/"
		}
		return fmt.Sprintf("%s: %s", path, ve.ErrorKind.LocalizedString(printer))
	}
	return err.Error()
}

func validateUniqueness(cfg *Config) error {
	ports := make(map[int]int)
	instances := make(map[string]int)

	for i, p := range cfg.Proxies {
		if prev, ok := ports[p.Port]; ok {
			return fmt.Errorf("Invalid config: proxies.%d.port: duplicate port %d (same as proxies.%d)", i, p.Port, prev)
		}
		ports[p.Port] = i

		if prev, ok := instances[p.Instance]; ok {
			return fmt.Errorf("Invalid config: proxies.%d.instance: duplicate instance %q (same as proxies.%d)", i, p.Instance, prev)
		}
		instances[p.Instance] = i
	}
	return nil
}
