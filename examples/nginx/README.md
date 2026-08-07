# Nginx Example

This example demonstrates how to build a declarative, secure Nginx container image using Doko without writing a Dockerfile.

## Running the Build

Run the following command to build the image:

```bash
docker buildx build -f doko.yaml --tag my-secure-nginx:latest --load .
```

## Features Demonstrated

1. **Security Hardening**: setting `accounts.root: false` completely purges the root user/group.
2. **Negative Packages**: trims unneeded/dangerous packages explicitly (e.g. `!telnet`).
3. **Ports**: specifies listening ports under the `runtime:` section.
