terraform {
  required_providers {
    skipjack = {
      source = "bitnixdev/skipjack"
    }
  }
}

provider "skipjack" {}

data "skipjack_secrets" "ci" {
  org     = "acme"
  project = "shared-ci"
}

output "secret_names" {
  value = data.skipjack_secrets.ci.names
}
