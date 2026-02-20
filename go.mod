module github.com/cloudblue/chaperone

go 1.25.7

// Pre-publication: SDK resolved via local path until tagged and published.
// Remove this directive after: git tag sdk/v1.0.0 && git push origin sdk/v1.0.0
// Then update require to: github.com/cloudblue/chaperone/sdk v1.0.0
// See ADR-004 in docs/explanation/DESIGN-SPECIFICATION.md
replace github.com/cloudblue/chaperone/sdk => ./sdk

require github.com/cloudblue/chaperone/sdk v0.0.0

require (
	github.com/prometheus/client_golang v1.23.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/sys v0.35.0 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
)
