// Copyright 2026 CloudBlue LLC
// SPDX-License-Identifier: Apache-2.0

package api

// Audit action constants logged for each portal operation.
const (
	AuditActionInstanceCreate = "instance.create"
	AuditActionInstanceUpdate = "instance.update"
	AuditActionInstanceDelete = "instance.delete"
	AuditActionUserLogin      = "user.login"
	AuditActionUserLogout     = "user.logout"
	AuditActionPasswordChange = "user.password_change"
)
