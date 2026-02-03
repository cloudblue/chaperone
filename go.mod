module github.com/cloudblue/chaperone

go 1.25.6

// LOCAL DEVELOPMENT ONLY
// This replace directive allows building without publishing the SDK.
//
// TODO(Phase 2 - Module Separation): Remove this replace directive when:
//   1. Repository is made public
//   2. SDK is tagged with semantic version (e.g., git tag sdk/v1.0.0)
//   3. Update require to real version: require github.com/cloudblue/chaperone/sdk v1.0.0
//
// See: docs/ROADMAP.md "Module Separation" task
// See: ADR-004 in docs/DESIGN-SPECIFICATION.md
replace github.com/cloudblue/chaperone/sdk => ./sdk

require github.com/cloudblue/chaperone/sdk v0.0.0

require gopkg.in/yaml.v3 v3.0.1
