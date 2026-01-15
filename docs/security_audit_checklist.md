# Security Audit Checklist (Local Filesystem)

This checklist covers the local filesystem storage model only. Use it during code reviews and periodic security assessments.

## 1. Path Traversal Prevention

### 1.1 Project ID Validation
- [ ] All project IDs pass through `ValidateProjectID()` before filesystem operations
- [ ] Rejects `..` and parent directory traversal patterns
- [ ] Rejects absolute paths (starting with `/` or `\`)
- [ ] Rejects Windows-style paths with backslashes
- [ ] Returns a single directory name only

### 1.2 Session ID Validation
- [ ] All session IDs pass through `ValidateSessionID()` before filesystem operations
- [ ] Session IDs cannot contain path separators

## 2. Concurrency & Race Conditions

### 2.1 Session Safety
- [ ] Concurrent checkouts do not race on shared state
- [ ] Workspace paths are deterministic and validated per request

### 2.2 Commit Semantics
- [ ] File writes are atomic where possible
- [ ] Failed writes do not leave partial files

## 3. Resource Cleanup

### 3.1 Workspace Cleanup
- [ ] `Cleanup()` is a no-op for local workspaces and does not delete project data
- [ ] No orphaned temporary directories remain after runs

### 3.2 File Handle Hygiene
- [ ] File handles are closed after reads/writes
- [ ] Context cancellation propagates correctly

## 4. Input Validation

### 4.1 User ID Validation
- [ ] User IDs validated as positive integers
- [ ] User ID 0 handled correctly (system/anonymous)

### 4.2 Configuration Validation
- [ ] `WORKDIR` is required and resolved to an absolute path
- [ ] Filesystem operations stay within `WORKDIR`

## 5. Logging & Audit

### 5.1 Security-Relevant Logging
- [ ] Failed validation attempts logged with context
- [ ] Successful checkouts logged with user/project/session

### 5.2 Sensitive Data Exclusion
- [ ] No file contents logged
- [ ] No user credentials logged

## 6. Denial of Service Prevention

### 6.1 Resource Limits
- [ ] Maximum file size limits enforced
- [ ] Maximum files per workspace limited
- [ ] Request timeouts configured

## 7. Network & Host Security

### 7.1 Host Isolation
- [ ] `WORKDIR` is not exposed via network shares
- [ ] Project directories are not world-writable

## 8. Periodic Security Tasks

### 8.1 Weekly
- [ ] Review failed validation logs
- [ ] Check for orphaned temp directories

### 8.2 Monthly
- [ ] Review access logs for anomalies
- [ ] Update dependencies for security patches

## Security Test Commands

```bash
# Path traversal tests
go test -v -run "Traversal|Validate" ./internal/workspaces/

# Race condition detection
go test -race ./internal/workspaces/
```
