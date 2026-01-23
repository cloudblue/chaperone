module github.com/cloudblue/chaperone

go 1.25.5

// Local development: reference the SDK from this repo
// Remove this replace directive when SDK is published
replace github.com/cloudblue/chaperone/sdk => ./sdk

require github.com/cloudblue/chaperone/sdk v0.0.0
