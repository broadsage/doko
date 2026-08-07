# jq Compiled From Source Example

This example demonstrates how to perform multi-stage sub-builds to build a binary from source (compiling `jq`) and export only the final executable into a hardened runtime environment.

## Running the Build

Run the following command to build the image:

```bash
docker buildx build -f doko.yaml --tag my-compiled-jq:latest --load .
```

## Features Demonstrated

1. **Multi-Stage Sub-Builds**: Uses `builds:` block to define isolated builder stages.
2. **Native Pipeline Templates**: Integrates pipelines and step execution (`runs:` script).
3. **Artifact Transfer**: Copies only the built binary from builder to final hardened stage.
