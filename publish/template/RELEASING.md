# Releasing __TITLE__

Versions follow [SemVer](https://semver.org): `vMAJOR.MINOR.PATCH`. Before 1.0 a minor bump may
break config or CLI; after 1.0 only a major bump may. Pre-releases are `v0.3.0-rc.1` and are marked
as such on GitHub automatically.

1. Make sure `main` is green (the `ci` workflow: tests, builds, GoReleaser check, bench smoke).
2. Move the `## [Unreleased]` notes in `CHANGELOG.md` under a new `## [X.Y.Z] - YYYY-MM-DD`
   heading — a short **Highlights** paragraph first, then Added / Changed / Fixed / Removed.
   Those notes open the GitHub Release; the commit list is generated below them.
3. Commit (`chore: release vX.Y.Z`), tag, push:

       git tag -a vX.Y.Z -m "vX.Y.Z" && git push origin main vX.Y.Z

4. The `release` workflow builds linux/darwin × amd64/arm64 archives, `checksums.txt`, the
   `ghcr.io/__OWNER_LC__/__DIST__` image (`:X.Y.Z`, `:latest` for non-pre-releases) and signed build
   provenance (`gh attestation verify <archive> --repo __OWNER__/__REPO__`).

Commit messages use [Conventional Commits](https://www.conventionalcommits.org) (`feat:`, `fix:`,
`docs:` …) so the generated list groups Features and Fixes.
