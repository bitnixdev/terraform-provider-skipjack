package provider

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bitnixdev/terraform-provider-skipjack/internal/client"
)

var (
	// Matches Skipjack slugSchema: lowercase letters, digits, dashes; starts alphanumeric.
	slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	// Matches Skipjack secretNameSchema: valid env var names.
	namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func scopeFromAttrs(org, project types.String) (client.Scope, diag.Diagnostics) {
	var diags diag.Diagnostics

	orgVal := org.ValueString()
	if orgVal == "" {
		diags.AddError("Invalid org", "org is required")
		return client.Scope{}, diags
	}
	if !slugPattern.MatchString(orgVal) {
		diags.AddError("Invalid org", "org must be a lowercase slug (letters, digits, dashes)")
		return client.Scope{}, diags
	}

	projectVal := ""
	if !project.IsNull() && !project.IsUnknown() {
		projectVal = project.ValueString()
		if projectVal != "" && !slugPattern.MatchString(projectVal) {
			diags.AddError("Invalid project", "project must be a lowercase slug (letters, digits, dashes)")
			return client.Scope{}, diags
		}
	}

	return client.Scope{Org: orgVal, Project: projectVal}, diags
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 128 {
		return fmt.Errorf("name must be at most 128 characters")
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name must be a valid environment variable name (^[A-Za-z_][A-Za-z0-9_]*$)")
	}
	return nil
}

func optionalStringPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}
