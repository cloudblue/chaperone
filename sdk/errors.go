// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package sdk

import "errors"

// ErrInvalidContextData indicates a transaction context field is present
// but fails validation (wrong type or empty string).
// Used by DataString and other context validation functions.
var ErrInvalidContextData = errors.New("invalid context data type")
