// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/url"
	"regexp"
)

// validTenantID matches Azure AD tenant identifiers: GUIDs, domain names
// (alphanumeric with dots and hyphens), or the literal "common"/"organizations"/
// "consumers". It rejects path separators, query strings, and fragments.
//
// Keep in sync with plugins/contrib/microsoft/token.go
var validTenantID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.\-]*$`)

// validateTenantID checks that tenant is a valid Azure AD tenant identifier.
func validateTenantID(tenant string) error {
	if tenant == "" {
		return fmt.Errorf("tenant ID is required")
	}
	if !validTenantID.MatchString(tenant) {
		return fmt.Errorf("invalid tenant ID %q: must be alphanumeric with dots/hyphens", tenant)
	}
	return nil
}

// validateURL checks that u is a valid URL with a scheme and host.
// It returns a boolean indicating whether the URL uses HTTPS (callers
// should warn on HTTP).
func validateURL(u string) error {
	if u == "" {
		return fmt.Errorf("URL is required")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", u, err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("URL %q must use HTTPS (or HTTP) scheme", u)
	}
	if parsed.Host == "" {
		return fmt.Errorf("URL %q has no host", u)
	}
	return nil
}

// isHTTPS returns true if the URL uses the HTTPS scheme.
func isHTTPS(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	return parsed.Scheme == "https"
}

// validateNonEmpty checks that value is not empty. The name parameter is used
// in the error message to identify the field.
func validateNonEmpty(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
