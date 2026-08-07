# Python API Example

This example demonstrates how to build a declarative, minimal Python API container image using Doko.

## Running the Build

Run the following command to build the image:

```bash
docker buildx build -f doko.yaml --tag my-python-api:latest --load .
```

## Features Demonstrated

1. **Minimal Runtime Environment**: Installs python3 without any package manager overhead or root access.
2. **Work-Dir and Ports**: configures runtime settings directly.
