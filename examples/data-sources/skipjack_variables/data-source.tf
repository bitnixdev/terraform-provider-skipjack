terraform {
  required_providers {
    skipjack = {
      source = "bitnixdev/skipjack"
    }
  }
}

provider "skipjack" {}

data "skipjack_variables" "ci" {
  org     = "acme"
  project = "shared-ci"
}

output "variable_names" {
  value = data.skipjack_variables.ci.names
}
