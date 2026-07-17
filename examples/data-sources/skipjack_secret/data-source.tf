terraform {
  required_providers {
    skipjack = {
      source = "bitnixdev/skipjack"
    }
  }
}

provider "skipjack" {}

data "skipjack_secret" "npm" {
  org     = "acme"
  project = "shared-ci"
  name    = "NPM_TOKEN"
}

output "version" {
  value = data.skipjack_secret.npm.version
}
