terraform {
  required_providers {
    skipjack = {
      source = "bitnixdev/skipjack"
    }
  }
}

provider "skipjack" {
  # token via SKIPJACK_TOKEN
}

resource "skipjack_secret" "npm" {
  org         = "acme"
  project     = "shared-ci"
  name        = "NPM_TOKEN"
  value       = var.npm_token
  description = "npm publish token for CI"
}

variable "npm_token" {
  type      = string
  sensitive = true
}
