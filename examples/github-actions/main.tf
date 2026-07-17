# Example: Terraform in GitHub Actions authenticated to Skipjack via OIDC.
#
# 1. In Skipjack, create a project OIDC workload identity, e.g.:
#      issuer:          https://token.actions.githubusercontent.com
#      audience:        https://skipjack.bitnix.dev   # or your deployment URL
#      subject_pattern: repo:acme/infra:*             # or a tighter pattern
#      permissions:     secrets/variables read/write as needed
#
# 2. Workflow needs:
#      permissions:
#        id-token: write
#        contents: read
#
# 3. No SKIPJACK_TOKEN required — the provider mints a GHA OIDC JWT automatically.

terraform {
  required_providers {
    skipjack = {
      source = "bitnixdev/skipjack"
    }
  }
}

provider "skipjack" {
  # url defaults to https://skipjack.bitnix.dev
  # oidc_audience defaults to url — must match the workload identity audience
}

data "skipjack_variables" "ci" {
  org     = "acme"
  project = "infra"
}

output "variable_names" {
  value = data.skipjack_variables.ci.names
}
