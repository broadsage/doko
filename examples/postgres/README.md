# PostgreSQL Example

This example demonstrates how to build a declarative, hardened PostgreSQL container image using Doko.

## Running the Build

Run the following command to build the image:

```bash
docker buildx build -f doko.yaml --tag my-secure-postgres:latest --load .
```

## Features Demonstrated

1. **Directories and Permissions**: Sets up directories under `/var/lib/postgresql/data` with specific `uid`, `gid`, and `mode`.
2. **Hardened Accounts**: Creates custom non-root user and group config for the database worker.
3. **Environment**: Populates necessary default database execution parameters.
