package doko.security

import rego.v1

# Helper to check if a list contains a specific value
has_element(list, val) if {
    list[_] == val
}

# Rule 1: Prevent running as Root (Error)
deny_errors contains {"id": "root-execution", "msg": msg} if {
    input.accounts.root == true
    msg := "'accounts.root' is set to true. Run container as a non-root user instead."
}

deny_errors contains {"id": "root-execution", "msg": msg} if {
    input.accounts["run-as"] == "root"
    msg := "'accounts.run-as' is set to 'root'. Avoid using root user in production."
}

deny_errors contains {"id": "root-execution", "msg": msg} if {
    not input.accounts["run-as"]
    not input.accounts.root == false
    msg := "'accounts.run-as' is empty, defaults to root. Explicitly specify a non-root user."
}

# Rule 2: Standard Non-Root UIDs (Warning)
deny_warnings contains {"id": "system-uid", "msg": msg} if {
    run_as := input.accounts["run-as"]
    user := input.accounts.users[_]
    user.name == run_as
    user.uid < 1000
    user.uid != 65532
    msg := sprintf("User '%s' has a system UID %d (< 1000). Use standard non-root UID (e.g. 65532).", [run_as, user.uid])
}

# Rule 3: Exclude Dev Tools in Production/Runtime Variants (Warning)
forbidden_packages := ["gcc", "g++", "make", "git", "go", "cargo", "npm", "pip"]

deny_warnings contains {"id": "dev-tools-in-production", "msg": msg} if {
    input.variant == "runtime"
    pkg := input.contents.packages[_]
    has_element(forbidden_packages, pkg)
    msg := sprintf("Development tool '%s' is present in a runtime image. Exclude compiling/development tools in production variants.", [pkg])
}

deny_warnings contains {"id": "dev-tools-in-production", "msg": msg} if {
    input.variant == "production"
    pkg := input.contents.packages[_]
    has_element(forbidden_packages, pkg)
    msg := sprintf("Development tool '%s' is present in a production image. Exclude compiling/development tools in production variants.", [pkg])
}

# Rule 4: Missing Metadata Annotations (Warning)
required_annotations := ["org.opencontainers.image.title", "org.opencontainers.image.description"]

deny_warnings contains {"id": "missing-annotations", "msg": msg} if {
    required := required_annotations[_]
    not input.annotations[required]
    msg := sprintf("Standard OCI annotation '%s' is missing under 'annotations'.", [required])
}

# Rule 5: Sensitive File Permissions (Warning)
deny_warnings contains {"id": "permissive-permissions", "msg": msg} if {
    path_config := input.contents.paths[_]
    path_config.mode == "0777"
    msg := sprintf("Path '%s' is configured with permissive permissions (mode: '0777'). Use restricted permissions (e.g. '0755' or '0750').", [path_config.path])
}

# Rule 6: Distroless Shell Enforcement in Production (Warning)
forbidden_shells := ["sh", "bash", "ash", "zsh", "apk", "apk-tools"]

deny_warnings contains {"id": "no-shells-in-production", "msg": msg} if {
    input.variant == "production"
    pkg := input.contents.packages[_]
    has_element(forbidden_shells, pkg)
    msg := sprintf("Development tool/shell '%s' is present in a production variant. Enforce distroless environments in production.", [pkg])
}

# Rule 7: Secrets Exposure in Config (Error)
sensitive_keys := ["password", "secret", "token", "apikey", "private_key"]

is_sensitive(key) if {
    s := sensitive_keys[_]
    contains(lower(key), s)
}

deny_errors contains {"id": "secrets-exposure", "msg": msg} if {
    val := input.environment[key]
    is_sensitive(key)
    msg := sprintf("Potential credential exposure: Key '%s' appears to contain sensitive information in 'environment'.", [key])
}

deny_errors contains {"id": "secrets-exposure", "msg": msg} if {
    val := input.runtime.env[key]
    is_sensitive(key)
    msg := sprintf("Potential credential exposure: Key '%s' appears to contain sensitive information in 'runtime.env'.", [key])
}

deny_errors contains {"id": "secrets-exposure", "msg": msg} if {
    val := input.vars[key]
    is_sensitive(key)
    msg := sprintf("Potential credential exposure: Key '%s' appears to contain sensitive information in 'vars'.", [key])
}

# Rule 8: Deprecated / Insecure Packages (Warning)
obsolete_packages := ["telnet", "ftp", "python2", "rsh"]

deny_warnings contains {"id": "deprecated-packages", "msg": msg} if {
    pkg := input.contents.packages[_]
    has_element(obsolete_packages, pkg)
    msg := sprintf("Obsolete package '%s' is deprecated and has known security alternatives. Replace with secure packages.", [pkg])
}

# Rule 9: Privileged Compiler Build Stages (Warning)
deny_warnings contains {"id": "privileged-stages", "msg": msg} if {
    build_stage := input.builds[_]
    build_stage.privileged == true
    msg := sprintf("Build stage '%s' is configured with 'privileged: true'. Restrict elevated compiler privileges only to verified steps.", [build_stage.name])
}
