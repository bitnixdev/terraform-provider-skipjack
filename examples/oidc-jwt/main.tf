# Example: supply an OIDC JWT from any CI system (Terraform Cloud, GitLab, …).
# Configure a matching Skipjack OIDC workload identity (issuer, audience, sub).

terraform {
  required_providers {
    skipjack = {
      source = "bitnixdev/skipjack"
    }
  }
}

provider "skipjack" {
  # Prefer env: export SKIPJACK_TOKEN="$(oidc-jwt-from-ci)"
  # token = var.oidc_jwt
}

variable "oidc_jwt" {
  type        = string
  sensitive   = true
  description = "Short-lived OIDC JWT; optional if SKIPJACK_TOKEN is set"
  default     = null
}

data "skipjack_secrets" "ci" {
  org     = "acme"
  project = "infra"
}
