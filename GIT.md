# Git Workflow for sb-manager-sdk

This repository contains the Slidebolt Manager SDK, which aggregates core SDKs and standard plugins to provide a unified development and testing environment. It is primarily used for integration testing and high-level ecosystem management.

## Dependencies
- **Internal:**
  - Core SDKs: `sb-contract`, `sb-domain`, `sb-messenger-sdk`, `sb-runtime`, `sb-storage-sdk`, `sb-storage-server`, `sb-logging-sdk`, `sb-logging`, `sb-script`, `sb-testkit`.
  - Standard Plugins: `plugin-system`, `plugin-automation`, `plugin-amcrest`, `plugin-androidtv`, `plugin-esphome`, `plugin-frigate`, `plugin-kasa`, `plugin-wiz`, `plugin-zigbee2mqtt`.
- **External:** 
  - Standard Go library and NATS.
  - AWS SDK (used by some integration components).

## Build Process
- **Type:** Pure Go Library (Composite SDK).
- **Consumption:** Imported as a module dependency in high-level management tools or complex integration tests.
- **Artifacts:** No standalone binary or executable is produced.
- **Validation:** 
  - Validated through comprehensive integration tests: `go test -v ./...`
  - Validated by its consumers during their respective ecosystem-wide test cycles.

## Pre-requisites & Publishing
As a composite SDK, `sb-manager-sdk` should be the **last** internal library updated in a release cycle, as it depends on nearly every other Slidebolt module.

**Before publishing:**
1. Determine current tag: `git tag | sort -V | tail -n 1`
2. Ensure all local tests pass: `go test -v ./...`
3. Verify that all child plugins and core SDKs have been published with their latest versions.

**Publishing Order:**
1. Ensure all internal dependencies are tagged and pushed.
2. Update `sb-manager-sdk/go.mod` to reference the latest tags for all 16+ dependencies.
3. Determine next semantic version for `sb-manager-sdk` (e.g., `v1.0.4`).
4. Commit and push the changes to `main`.
5. Tag the repository: `git tag v1.0.4`.
6. Push the tag: `git push origin main v1.0.4`.

## Update Workflow & Verification
1. **Modify:** Update composite logic or unified testing helpers.
2. **Verify Local:**
   - Run `go mod tidy`.
   - Run `go test ./...`.
3. **Commit:** Ensure the commit message lists major dependency updates.
4. **Tag & Push:** (Follow the Publishing Order above).
