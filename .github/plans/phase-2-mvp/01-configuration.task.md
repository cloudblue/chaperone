# Task: Configuration System

**Status:** [x] Completed
**Priority:** P0
**Estimated Effort:** M

## Objective

Implement YAML configuration file loading with environment variable overrides following 12-Factor App methodology.

## Design Spec Reference

- **Primary:** Section 5.5.A - Configuration File Structure
- **Primary:** Section 5.5.B - Environment Variable Overrides
- **Related:** ADR-005 - Decoupled Configuration & Naming

## Dependencies

- [x] Phase 1 complete - Core proxy skeleton exists
- [x] None within Phase 2 (this is the foundation task)

## Acceptance Criteria

- [x] `config.yaml` loads from configurable path (default: `./config.yaml`, override via `CHAPERONE_CONFIG`)
- [x] All config sections supported: `server`, `upstream`, `observability`
- [x] Environment variables override YAML values using `CHAPERONE_<SECTION>_<KEY>` pattern
- [x] Nested config uses underscore separator (e.g., `CHAPERONE_SERVER_TLS_CERT_FILE`)
- [x] Missing required config returns clear error with field name
- [x] Default values applied for optional fields
- [x] Config validation rejects invalid values (bad ports, missing files, etc.)
- [x] Tests pass: `go test ./internal/config/...`
- [x] Lint passes: `make lint`

## Implementation Hints

### Suggested Approach

1. Create `internal/config/config.go` with struct definitions matching Design Spec §5.5.A
2. Create `internal/config/loader.go` for YAML + env var loading logic
3. Use `gopkg.in/yaml.v3` (already approved dependency)
4. Implement env var override using reflection or explicit mapping
5. Add validation for required fields and value constraints
6. Integrate with `cmd/chaperone/main.go`

### Config Struct (from Design Spec)

```go
type Config struct {
    Server       ServerConfig       `yaml:"server"`
    Upstream     UpstreamConfig     `yaml:"upstream"`
    Observability ObservabilityConfig `yaml:"observability"`
}

type ServerConfig struct {
    Addr      string    `yaml:"addr"`       // ":443"
    AdminAddr string    `yaml:"admin_addr"` // ":9090"
    TLS       TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
    CertFile   string `yaml:"cert_file"`
    KeyFile    string `yaml:"key_file"`
    CAFile     string `yaml:"ca_file"`
    AutoRotate bool   `yaml:"auto_rotate"`
}

type UpstreamConfig struct {
    HeaderPrefix string              `yaml:"header_prefix"` // "X-Connect"
    TraceHeader  string              `yaml:"trace_header"`  // "Connect-Request-ID"
    AllowList    map[string][]string `yaml:"allow_list"`
    Timeouts     TimeoutConfig       `yaml:"timeouts"`
}

type TimeoutConfig struct {
    Connect time.Duration `yaml:"connect"` // 5s
    Read    time.Duration `yaml:"read"`    // 30s
    Write   time.Duration `yaml:"write"`   // 30s
    Idle    time.Duration `yaml:"idle"`    // 120s
}

type ObservabilityConfig struct {
    LogLevel         string   `yaml:"log_level"`         // "info"
    EnableProfiling  bool     `yaml:"enable_profiling"`  // false
    SensitiveHeaders []string `yaml:"sensitive_headers"` // redaction list
}
```

### Key Code Locations

- `internal/config/config.go` - Struct definitions
- `internal/config/loader.go` - Load + merge logic
- `internal/config/defaults.go` - Default values
- `internal/config/validate.go` - Validation rules
- `cmd/chaperone/main.go` - Integration point

### Gotchas

- Duration parsing: YAML duration strings need custom unmarshaler or `time.ParseDuration`
- Env var precedence: Environment must override YAML, not vice versa
- Sensitive defaults: `sensitive_headers` should have secure defaults even if not configured
- Empty allow_list should default to deny-all (secure default)

## Files to Create/Modify

- [x] `internal/config/config.go` - Config struct definitions
- [x] `internal/config/loader.go` - YAML + env loading
- [x] `internal/config/defaults.go` - Default value constants
- [x] `internal/config/validate.go` - Validation logic
- [x] `internal/config/config_test.go` - Unit tests
- [x] `configs/config.example.yaml` - Example configuration file
- [x] `cmd/chaperone/main.go` - Load config at startup
- [x] `cmd/chaperone/main_test.go` - Config integration tests
- [x] `README.md` - Exhaustive configuration documentation

## Testing Strategy

- **Unit tests:** 
  - YAML parsing with various inputs
  - Env var override precedence
  - Validation error cases
  - Default value application
- **Table-driven tests:** Multiple config scenarios
- **Integration tests:** Config loading in main_test.go
- **Fuzz tests:** Malformed YAML inputs (per Design Spec §9.2.B)
