# Redis Example

This example demonstrates how to build a declarative, secure Redis cache/database container image using Doko.

## Running the Build

Run the following command to build the image:

```bash
docker buildx build -f doko.yaml --tag my-secure-redis:latest --load .
```

## Features Demonstrated

1. **Redis Server deployment**: Builds the database package natively and sets default config paths.
2. **Environment Variables**: Sets standard execution parameters for the nonroot runtime user.
