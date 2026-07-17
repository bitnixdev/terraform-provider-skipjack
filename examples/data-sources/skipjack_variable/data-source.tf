terraform {
  required_providers {
    skipjack = {
      source = "bitnixdev/skipjack"
    }
  }
}

provider "skipjack" {}

data "skipjack_variable" "node_env" {
  org     = "acme"
  project = "shared-ci"
  name    = "NODE_ENV"
}

output "node_env" {
  value = data.skipjack_variable.node_env.value
}
