# Terraform Provider for Skipjack

Manage [Skipjack](https://github.com/bitnixdev/skipjack-backend) secrets and
variables from Terraform using the machine-facing **token API** (`/v1`).

Authentication is either:

- a long-lived **API key** (`sjk_...`), or
- a short-lived **OIDC JWT** for a configured [workload identity](https://github.com/bitnixdev/skipjack-backend)
  (GitHub Actions, Terraform Cloud, GitLab, …)

This provider intentionally covers a **subset** of the HashiCorp Vault
provider’s secret surface — enough to read and write named secrets/variables
in an org or project scope, without Vault’s engines, auth methods, policies,
or dynamic credential backends.

| Vault concept | Skipjack equivalent |
| --- | --- |
| `provider "vault" { address, token }` | `provider "skipjack" { url, token }` |
| AppRole / JWT auth for CI | OIDC workload identity → same `/v1` Bearer |
| `vault_kv_secret_v2` / `vault_generic_secret` resource | `skipjack_secret` |
| KV data source for one path | `data.skipjack_secret` |
| `vault_kv_secrets_list` + read | `data.skipjack_secrets` (names + values) |
| Non-secret config (no direct Vault analog) | `skipjack_variable` / data sources |

## Requirements

- [Terraform](https://www.terraform.io/downloads) >= 1.0
- [Go](https://go.dev/dl/) >= 1.24 (to build from source)
- Either:
  - a Skipjack API key with the relevant `secretsRead` / `secretsWrite` /
    `variablesRead` / `variablesWrite` permissions, **or**
  - an OIDC workload identity on the org/project with matching issuer,
    audience, subject pattern, scopes, and permissions

## Provider configuration

```hcl
terraform {
  required_providers {
    skipjack = {
      source  = "bitnixdev/skipjack"
      version = "~> 0.1"
    }
  }
}

provider "skipjack" {
  # Optional. Defaults to https://skipjack.bitnix.dev
  # url = "https://skipjack.example.com"

  # API key or OIDC JWT. Prefer SKIPJACK_TOKEN in the environment.
  # token = var.skipjack_token

  # Audience for GitHub Actions auto OIDC (defaults to url).
  # oidc_audience = "https://skipjack.example.com"
}
```

| Argument | Env var | Default | Description |
| --- | --- | --- | --- |
| `url` | `SKIPJACK_URL` | `https://skipjack.bitnix.dev` | Skipjack origin (HTTPS). |
| `token` | `SKIPJACK_TOKEN` | *(see auth below)* | API key (`sjk_...`) **or** OIDC JWT. |
| `oidc_audience` | `SKIPJACK_OIDC_AUDIENCE` | provider `url` | Audience when minting a GitHub Actions OIDC token. |

### Authentication

Credential resolution order:

1. `token` attribute
2. `SKIPJACK_TOKEN` environment variable
3. **GitHub Actions OIDC auto-fetch** when `ACTIONS_ID_TOKEN_REQUEST_URL` and
   `ACTIONS_ID_TOKEN_REQUEST_TOKEN` are set (the runner injects these when the
   job has `permissions: id-token: write`)

Both API keys and OIDC JWTs are sent as `Authorization: Bearer …` on `/v1`.
Skipjack decides which principal matches.

#### Local / long-lived key

```sh
export SKIPJACK_TOKEN=sjk_...
terraform plan
```

#### GitHub Actions (no static key)

Create a **project or org OIDC workload identity** in Skipjack, for example:

| Field | Example |
| --- | --- |
| Issuer | `https://token.actions.githubusercontent.com` |
| Audience | `https://skipjack.bitnix.dev` (must match `oidc_audience` / `url`) |
| Subject pattern | `repo:acme/infra:*` (or a tighter glob) |
| Permissions | e.g. `variablesRead`, `secretsRead` |

```yaml
# .github/workflows/terraform.yml
permissions:
  id-token: write
  contents: read

jobs:
  plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: hashicorp/setup-terraform@v3
      - run: terraform init && terraform plan
        # No SKIPJACK_TOKEN — provider mints a GHA OIDC JWT automatically.
```

See `examples/github-actions/`.

#### Other CI (Terraform Cloud, GitLab, …)

Mint a JWT from your platform and pass it as `SKIPJACK_TOKEN` (or `token`).
Configure a matching Skipjack OIDC workload identity for that issuer/audience/subject.
See `examples/oidc-jwt/`.

> Note: this is **not** the policy-gated `POST /oidc/secrets` exchange used by
> the GitHub Action for exporting env vars. Workload identities authorize the
> full `/v1` secret/variable API (subject to the identity’s permission flags).

## Resources

### `skipjack_secret`

Creates or updates a secret (`PUT /v1/orgs/:org[/projects/:proj]/secrets/:name`).

```hcl
resource "skipjack_secret" "npm" {
  org     = "acme"
  project = "shared-ci" # omit for org-level
  name    = "NPM_TOKEN"
  value   = var.npm_token

  description = "npm publish token"
}

# Import: terraform import skipjack_secret.npm 'acme/shared-ci/NPM_TOKEN'
# Org-level: terraform import skipjack_secret.org 'acme//ORG_SECRET'
```

| Argument | Required | Description |
| --- | --- | --- |
| `org` | yes | Organization slug. Forces new. |
| `project` | no | Project slug; omit for org-level. Forces new. |
| `name` | yes | Env-var style name. Forces new. |
| `value` | yes | Secret value (sensitive; stored in state). |
| `description` | no | Optional description. |

| Attribute | Description |
| --- | --- |
| `id` | `org/project/name` or `org//name`. |
| `version` | Server-side version after the last write. |

### `skipjack_variable`

Same shape as `skipjack_secret`, but for non-secret config values
(`PUT .../variables/:name`). Values are not marked sensitive.

```hcl
resource "skipjack_variable" "node_env" {
  org     = "acme"
  project = "shared-ci"
  name    = "NODE_ENV"
  value   = "production"
}
```

## Data sources

### `skipjack_secret` / `skipjack_variable`

Read a single named secret or variable.

```hcl
data "skipjack_secret" "npm" {
  org     = "acme"
  project = "shared-ci"
  name    = "NPM_TOKEN"
}

output "npm_token" {
  value     = data.skipjack_secret.npm.value
  sensitive = true
}
```

### `skipjack_secrets` / `skipjack_variables`

List everything in a scope (names + values map), analogous to listing a Vault
KV path and reading entries.

```hcl
data "skipjack_secrets" "ci" {
  org     = "acme"
  project = "shared-ci"
}

data "skipjack_variables" "ci" {
  org     = "acme"
  project = "shared-ci"
}
```

## Installing from the Terraform Registry

Once [published](docs/publishing.md), use the provider from any project without
building it yourself:

```hcl
terraform {
  required_providers {
    skipjack = {
      source  = "bitnixdev/skipjack"
      version = "~> 0.1"
    }
  }
}
```

```sh
terraform init
```

Releases are built by GitHub Actions (GoReleaser) when you push a `v*` tag.
See **[docs/publishing.md](docs/publishing.md)** for GPG secrets, first release,
and Registry registration (`bitnixdev/skipjack`).

## Building and installing locally

```sh
make install   # builds and installs into ~/.terraform.d/plugins/...
make test      # unit tests (httptest mock of /v1)
```

Then point Terraform at the local plugin with a `dev_overrides` block in
`~/.terraformrc` if you prefer not to use the filesystem mirror layout:

```hcl
provider_installation {
  dev_overrides {
    "bitnixdev/skipjack" = "/path/to/terraform-provider-skipjack"
  }
  direct {}
}
```

## Security notes

- **Secret values are stored in Terraform state** when managed as resources or
  read via data sources. Protect state (remote backend encryption, limited
  access) the same way you would with `vault_kv_secret_v2`.
- Prefer `SKIPJACK_TOKEN` over putting the API key in `.tf` files.
- The token API only exposes secrets/variables the key is authorized for;
  unauthorized scopes return 404 (same as the server).

## Token API surface used

| Method | Path |
| --- | --- |
| `GET` | `/v1/orgs/:org[/projects/:proj]/secrets` |
| `GET` | `/v1/orgs/:org[/projects/:proj]/secrets/:name` |
| `PUT` | `/v1/orgs/:org[/projects/:proj]/secrets/:name` |
| `DELETE` | `/v1/orgs/:org[/projects/:proj]/secrets/:name` |
| `GET` | `/v1/orgs/:org[/projects/:proj]/variables` |
| `GET` | `/v1/orgs/:org[/projects/:proj]/variables/:name` |
| `PUT` | `/v1/orgs/:org[/projects/:proj]/variables/:name` |
| `DELETE` | `/v1/orgs/:org[/projects/:proj]/variables/:name` |

Auth header: `Authorization: Bearer sjk_...`.

## Out of scope (for now)

Compared to the full Vault provider (or Skipjack’s session admin API), this
provider does **not** manage:

- Organizations, projects, groups, members, invites
- Access policies / tags
- API key lifecycle
- OIDC exchange (that’s the GitHub Action’s job)
- Ephemeral/write-only secret attributes (possible future improvement)

## License

MPL-2.0 (same spirit as HashiCorp Terraform providers).
