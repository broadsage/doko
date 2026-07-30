# Advanced BuildKit Native Features in Doko

Doko leverages native features of the BuildKit Low-Level Builder (LLB) engine rather than parsing down to a Dockerfile. This document explains the advanced features and configurations.

---

## 1. Build-Time Arguments (`--build-arg`)
Doko supports passing build-time variables dynamically via the standard `--build-arg` options from client builders (like `docker buildx` or `buildctl`):

```bash
docker buildx build --build-arg PG_VERSION=18.1 -t my-app .
```

These values will be parsed from the gateway options (`build-arg:KEY`) and automatically merged into the `vars` configuration block of your `doko.yaml`.

---

## 2. SSH Agent Forwarding
You can forward your host machine's SSH agent socket into specific pipeline execution layers to securely authenticate against private resources (such as private Git repositories or package indexes) without copying private keys into the image.

### Configuration (`doko.yaml`)
To enable SSH agent forwarding for a pipeline step, set the `ssh: true` boolean property:

```yaml
contents:
  pipeline:
    - name: install-private-dependency
      runs: |
        ssh-keyscan github.com >> ~/.ssh/known_hosts
        git clone git@github.com:my-org/private-repo.git
      ssh: true
```

### Building with SSH Agent
Invoke the build using the `--ssh` option:

```bash
docker buildx build --ssh default=$SSH_AUTH_SOCK -t my-app .
```

The SSH agent socket will be dynamically mounted at `/run/ssh-agent.sock` and the `SSH_AUTH_SOCK` environment variable will be populated automatically during that step's execution.

---

## 3. Dynamic Keyring and CA Certificate Mounts
Doko does not copy CA certificates (`contents.ca-certificates`) or keyrings (`contents.keyring`) into permanent image layers. 

Instead, they are mounted as **read-only transient mounts** (`llb.Readonly` with `llb.SourcePath`) from independent source states:
- They are mounted to `/etc/apk/keys/` *only during package management* (`apk`).
- Custom CA certificates are mounted *only during package management* and *only during the `update-ca-certificates` run*.

This ensures that:
- Raw certificate and key files are never written to the final image filesystem.
- Zero credentials or public key assets leak into OCI layers.
- Image sizes are kept strictly minimal.
