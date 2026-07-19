# Publishing to the Terraform Registry

This provider is set up for [HashiCorp Terraform Registry](https://registry.terraform.io)
publishing via GitHub Releases + GoReleaser. Once published, any of your
projects can depend on it with:

```hcl
terraform {
  required_providers {
    skipjack = {
      source  = "bitnixdev/skipjack"
      version = ">= 2026.1.0"
    }
  }
}
```

No local `make install` or `dev_overrides` required.

## Version scheme

Versions are **calendar versions**, assigned automatically:

```
YYYY.MM.DD.<revid>
```

| Part | Source |
| --- | --- |
| `YYYY.MM.DD` | UTC date of the release workflow run |
| `revid` | `git rev-list --count HEAD` (commit count at the released tip) |

Examples: `2026.07.19.12`, `2026.12.01.48`.

**You only push to `master`.** Do not create version tags yourself. The Release
workflow computes the version, creates the GitHub Release (and the version tag
the Registry expects), and publishes assets. Docs-only / example-only pushes
are ignored (`paths-ignore` in `.github/workflows/release.yml`).

Why a tag exists at all: the [Terraform Registry](https://developer.hashicorp.com/terraform/registry/providers/publishing)
discovers provider versions from **GitHub Releases**, and a GitHub Release is
always bound to a `v…` tag. That tag is an implementation detail of CI, not a
step in your workflow.

Local builds use the same version scheme via the Makefile
(`make version` / `make install`).

## What the automation does

| Piece | Role |
| --- | --- |
| `.github/workflows/release.yml` | On push to `master`: compute version, create release tag, build, sign, upload GitHub Release |
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

Push to `master` (with the Release workflow and GPG secrets in place). CI will
version, build, sign, and publish a GitHub Release automatically.

To re-run without a new commit: **Actions → Release → Run workflow**.

Assets look like:

```
terraform-provider-skipjack_2026.07.19.12_linux_amd64.zip
terraform-provider-skipjack_2026.07.19.12_darwin_arm64.zip
...
terraform-provider-skipjack_2026.07.19.12_SHA256SUMS
terraform-provider-skipjack_2026.07.19.12_SHA256SUMS.sig
terraform-provider-skipjack_2026.07.19.12_manifest.json
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
      version = ">= 2026.1.0"
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

Merge or push to `master`. Each qualifying push gets a new
`YYYY.MM.DD.<revid>` GitHub Release; the Registry picks it up once the provider
is linked. No manual versioning or tagging.

```sh
# Preview the version string CI would assign right now:
make version
```

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
| Release skipped (version already published) | Same UTC day + same commit count (e.g. re-run with no new commits); push another commit |

## Local development (still works)

For unreleased changes while hacking on the provider:

```sh
make install
# or ~/.terraformrc dev_overrides for "bitnixdev/skipjack"
```
