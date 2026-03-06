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
// By default it requires HTTPS; set allowHTTP to true for testing with
// local mock servers (the -allow-http flag).
func validateURL(u string, allowHTTP bool) error {
	if u == "" {
		return fmt.Errorf("URL is required")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", u, err)
	}
	if allowHTTP {
		if parsed.Scheme != "https" && parsed.Scheme != "http" {
			return fmt.Errorf("URL %q must use HTTPS or HTTP scheme", u)
		}
	} else {
		if parsed.Scheme != "https" {
			return fmt.Errorf("URL %q must use HTTPS scheme", u)
		}
	}
	if parsed.Host == "" {
		return fmt.Errorf("URL %q has no host", u)
	}
	return nil
}

// validateNonEmpty checks that value is not empty. The name parameter is used
// in the error message to identify the field.
func validateNonEmpty(name, value string) error { //nolint:unparam // name varies by call site in future subcommands
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
