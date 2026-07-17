terraform {
  required_providers {
    skipjack = {
      source = "bitnixdev/skipjack"
    }
  }
}

provider "skipjack" {}

resource "skipjack_variable" "node_env" {
  org     = "acme"
  project = "shared-ci"
  name    = "NODE_ENV"
  value   = "production"
}
