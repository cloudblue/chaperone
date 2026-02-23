module github.com/cloudblue/chaperone/plugins/contrib

go 1.25.7

// Pre-publication: SDK resolved via local path until tagged and published.
// Remove this directive after: git tag sdk/v1.0.0 && git push origin sdk/v1.0.0
// Then update require to: github.com/cloudblue/chaperone/sdk v1.0.0
// See ADR-004 in docs/explanation/DESIGN-SPECIFICATION.md
replace github.com/cloudblue/chaperone/sdk => ../../sdk

require github.com/cloudblue/chaperone/sdk v0.0.0

require golang.org/x/sync v0.19.0
