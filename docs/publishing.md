# Publishing to the Terraform Registry

This provider is set up for [HashiCorp Terraform Registry](https://registry.terraform.io)
publishing via GitHub Releases + GoReleaser. Once published, any of your
projects can depend on it with:

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

No local `make install` or `dev_overrides` required.

## What the automation does

| Piece | Role |
| --- | --- |
| `.github/workflows/release.yml` | On `v*` tags: build, sign, upload GitHub Release assets |
| `.goreleaser.yml` | Cross-compile zip archives + SHA256SUMS + registry manifest |
| `terraform-registry-manifest.json` | Declares protocol 6.0 (Plugin Framework) |
| `.github/workflows/test.yml` | CI build/test on push and PR |

Registry consumers download the matching OS/arch zip from the GitHub Release
and verify the GPG-signed checksums.

## One-time setup

### 1. GPG signing key

The Registry **requires** signed checksums. Create a dedicated key (not your
personal day-to-day key if you prefer):

```sh
gpg --full-generate-key
# RSA and RSA, 4096 bits, no expiry (or long), real name e.g. "bitnixdev Terraform"
# Note the key fingerprint printed at the end (40 hex chars).

# Export for GitHub Actions (armor + private key):
gpg --armor --export-secret-keys '<FINGERPRINT>' > private.gpg
# Public key for the Registry:
gpg --armor --export '<FINGERPRINT>' > public.gpg
```

Add **repository secrets** on `bitnixdev/terraform-provider-skipjack`
(Settings → Secrets and variables → Actions):

| Secret | Value |
| --- | --- |
| `GPG_PRIVATE_KEY` | Full contents of `private.gpg` |
| `PASSPHRASE` | Passphrase for that key (empty string secret if none) |

Delete local `private.gpg` after storing it in secrets.

### 2. Publish the first GitHub Release

Push the default branch, then create a semver tag:

```sh
# Ensure the release workflow is on the default branch first.
git tag v0.1.0
git push origin v0.1.0
```

(or with jj: tag the desired revision and push). The **Release** workflow
creates a GitHub Release with assets named like:

```
terraform-provider-skipjack_0.1.0_linux_amd64.zip
terraform-provider-skipjack_0.1.0_darwin_arm64.zip
...
terraform-provider-skipjack_0.1.0_SHA256SUMS
terraform-provider-skipjack_0.1.0_SHA256SUMS.sig
terraform-provider-skipjack_0.1.0_manifest.json
```

### 3. Register the provider on registry.terraform.io

1. Sign in at [registry.terraform.io](https://registry.terraform.io) with the
   GitHub account that owns (or can admin) the `bitnixdev` org.
2. **Publish → Provider** and select this repository
   (`bitnixdev/terraform-provider-skipjack`).
3. Namespace / name should resolve to **`bitnixdev/skipjack`**
   (binary/repo name `terraform-provider-skipjack` → type `skipjack`).
4. Paste the **public** GPG key (`public.gpg`) when prompted.
5. After the first release is detected, versions appear under
   https://registry.terraform.io/providers/bitnixdev/skipjack

If the GitHub repo is private, the Registry cannot list it for the public
catalog; for your own use a **public** repo is the straightforward path.
(There are org/private options on paid Registry tiers; public is simplest.)

### 4. Use it from any project

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
  # token via SKIPJACK_TOKEN or GHA OIDC
}
```

```sh
terraform init
```

## Subsequent releases

```sh
git tag v0.1.1   # or v0.2.0, etc.
git push origin v0.1.1
```

Bump version tags only; GoReleaser and the Registry pick up new GitHub Releases
automatically once the provider is linked.

## Local dry-run (optional)

```sh
# Install goreleaser: https://goreleaser.com/install/
export GPG_FINGERPRINT='<your fingerprint>'
goreleaser release --snapshot --clean   # builds locally, does not publish
```

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Release workflow fails at GPG import | `GPG_PRIVATE_KEY` / `PASSPHRASE` secrets |
| Registry never shows a version | Release assets must include `*_SHA256SUMS`, `*_SHA256SUMS.sig`, and `*_manifest.json` |
| `terraform init` 404 | Wrong `source` (must be `bitnixdev/skipjack`), or version not yet published |
| Signature verification failed | Public key on Registry must match the private key used in Actions |

## Local development (still works)

For unreleased changes while hacking on the provider:

```sh
make install
# or ~/.terraformrc dev_overrides for "bitnixdev/skipjack"
```
