# Release integrity and provenance

The first supported distribution channel is the public GitHub Release for a
`v*` tag promoted to `main`. Homebrew and Scoop are deferred until the
`acthurhq/homebrew-tap` and `acthurhq/scoop-bucket` repositories exist and have
dedicated publishing credentials. Their absence cannot fail a GitHub release.

## Published evidence

GoReleaser publishes Linux and macOS archives, a Windows zip, a SHA-256
manifest, and an SPDX JSON SBOM for every archive. GitHub Actions then provides
two independent origin proofs:

1. Cosign signs the checksum manifest without a long-lived key. The OIDC
   certificate binds the signature to the GitHub Actions workflow identity;
   its Sigstore bundle includes the certificate and Rekor transparency-log
   evidence.
2. GitHub artifact attestations bind every archive, checksum manifest, and
   SBOM to the repository, workflow, commit, and build invocation.

The release job receives only the job-scoped `GITHUB_TOKEN` and OIDC token. No
private signing key or signing password is stored in repository secrets.

## Consumer verification

Download the checksum manifest and its `.sigstore.json` bundle from the same
release. Verify the workflow identity before trusting the manifest:

```sh
cosign verify-blob \
  --bundle acthur_VERSION_checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/acthurhq/acthur/.github/workflows/ci.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  acthur_VERSION_checksums.txt

sha256sum --ignore-missing -c acthur_VERSION_checksums.txt
gh attestation verify acthur_VERSION_OS_ARCHIVE -R acthurhq/acthur
```

The Unix and Windows installers fail closed on a missing checksum entry or
checksum mismatch before extraction, and verify that the installed executable
reports the requested release version. Installer automation currently verifies
checksum integrity rather than Sigstore identity; security-sensitive consumers
should perform the Cosign and GitHub attestation checks above.

## Release requirements

- Tags are created only from the reviewed `main` promotion commit.
- The CI test matrix and native installer tests must pass before release.
- The GoReleaser dry run must build every archive and SBOM on non-tag changes.
- GoReleaser uploads a draft first; only successful signing and attestations
  allow CI to make it public. A failed SBOM, signing, attestation, or upload
  leaves no public release.
- Released tags and artifacts are immutable; corrections use a new version.
