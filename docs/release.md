# Doko Release Process

## Automated releases

Doko uses an automated release process via GitHub Actions. Releases are handled by the [`Release` action](https://github.com/broadsage/doko/actions/workflows/release.yaml).

### Scheduled releases

The release workflow runs automatically every Monday at 00:00 UTC. It will:
1. Check if there have been any changes since the last release
2. Automatically bump the patch version if changes are detected
3. Create a new tag and release with generated release notes
4. Build and attach release artifacts using GoReleaser

### Manual releases

To trigger a release manually:

1. Go to the [Release workflow](https://github.com/broadsage/doko/actions/workflows/release.yaml)
2. Click **"Run workflow"**
3. Select the branch (usually `main`)
4. Click **"Run workflow"**

The workflow will automatically determine if a release is needed and create one if there have been changes since the last release.

### Monitor the release automation

The release job takes 20 to 30 minutes to execute and will:
- Create a new patch version tag
- Generate release notes
- Build and sign release artifacts with Cosign
- Publish the release to GitHub

Once the `Release` action has completed successfully, find your release on
the [releases page](https://github.com/broadsage/doko/releases)

You can make editorial changes to the release notes after automation completes if needed.

## Minor releases

Occasionally there are large or breaking changes to Doko that we want to highlight with a new minor release.
A minor release should be cut shortly after a breaking change is made, so that regular patch releases don't release breaking changes.

The process for cutting a release is exactly the same as above, except that you should pick a new minor version.

For example, if the latest version is `v0.9.5`, create a minor release `v0.10.0`.