# Security Audit Checklist

This document provides a comprehensive security audit checklist for the workspace and project management subsystems. Use this checklist during code reviews, penetration testing, and periodic security assessments.

## 1. Path Traversal Prevention

### 1.1 Project ID Validation
- [ ] All project IDs pass through `ValidateProjectID()` before filesystem operations
- [ ] Rejects `..` and parent directory traversal patterns
- [ ] Rejects absolute paths (starting with `/` or `\`)
- [ ] Rejects Windows-style paths with backslashes
- [ ] Rejects hidden directories with traversal (e.g., `.hidden/../`)
- [ ] Returns sanitized path component (single directory name only)

**Test cases:**
```go
// Should REJECT:
"../etc"
"..\\windows"
"/etc/passwd"
"foo/../bar"
".."
"."
".hidden/../secret"

// Should ACCEPT:
"my-project"
"project-123"
"123e4567-e89b-12d3-a456-426614174000"
```

### 1.2 Session ID Validation
- [ ] All session IDs pass through `ValidateSessionID()` before filesystem operations
- [ ] Same traversal checks as project ID
- [ ] Session IDs cannot contain path separators

### 1.3 Object Key Validation
- [ ] S3 object keys are validated before local filesystem writes
- [ ] `EphemeralManager.hydrate()` rejects keys with traversal patterns
- [ ] Object keys are stripped of their prefix before local path construction

## 2. Encryption at Rest

### 2.1 Enterprise Mode Encryption
- [ ] Enterprise workspaces use encrypted backing storage
- [ ] Encryption keys are rotated per configuration policy
- [ ] Key material never logged or exposed in error messages
- [ ] tmpfs mounts used for ephemeral decrypted data

### 2.2 Key Provider Security
- [ ] Vault Transit provider uses short-lived tokens
- [ ] AWS KMS provider uses IAM roles (not hardcoded credentials)
- [ ] File provider keys have restrictive permissions (0600)

**Verify key file permissions:**
```bash
ls -la /path/to/key/file
# Should show: -rw------- (0600)
```

## 3. Object Storage Security

### 3.1 S3/MinIO Access Control
- [ ] Bucket policies restrict access to authenticated principals only
- [ ] No public bucket access enabled
- [ ] Server-side encryption enabled (SSE-S3 or SSE-KMS)
- [ ] Versioning enabled for audit trail

### 3.2 Presigned URL Safety
- [ ] No presigned URLs for write operations without authentication
- [ ] Short expiration times for any presigned URLs
- [ ] Presigned URLs scoped to specific objects (not bucket-wide)

## 4. Concurrency & Race Conditions

### 4.1 Session Locking
- [ ] Concurrent checkouts to same session are serialized
- [ ] Generation counters prevent stale overwrites
- [ ] Redis locks used in distributed deployments

### 4.2 Commit Atomicity
- [ ] Commits upload all files before updating metadata
- [ ] Failed commits don't leave partial state
- [ ] Generation incremented atomically

**Test concurrent commits:**
```bash
# Run concurrent commit test
go test -race -run TestConflictHandling ./internal/workspaces/
```

## 5. Resource Cleanup

### 5.1 Workspace Cleanup
- [ ] `Cleanup()` removes all temporary files
- [ ] No orphaned directories after session ends
- [ ] Cleanup handles partial/failed checkouts

### 5.2 Memory/Handle Leaks
- [ ] File handles closed after S3 downloads
- [ ] Context cancellation propagates correctly
- [ ] Redis connections pooled and released

## 6. Input Validation

### 6.1 User ID Validation
- [ ] User IDs validated as positive integers
- [ ] User ID 0 handled correctly (system/anonymous)
- [ ] No SQL injection in user ID handling

### 6.2 Configuration Validation
- [ ] Invalid workspace modes rejected
- [ ] S3 configuration validated before use
- [ ] Encryption settings validated at startup

## 7. Logging & Audit

### 7.1 Security-Relevant Logging
- [ ] Failed validation attempts logged with context
- [ ] Successful checkouts logged with user/project/session
- [ ] Commits logged with file counts

### 7.2 Sensitive Data Exclusion
- [ ] No file contents logged
- [ ] No encryption keys logged
- [ ] No user credentials logged
- [ ] No full S3 paths with sensitive project names

**Verify no sensitive data in logs:**
```bash
grep -r "key=" logs/ | wc -l  # Should be 0
grep -r "password" logs/ | wc -l  # Should be 0
```

## 8. Denial of Service Prevention

### 8.1 Resource Limits
- [ ] Maximum file size limits enforced
- [ ] Maximum files per workspace limited
- [ ] Checkout timeout configured

### 8.2 Rate Limiting
- [ ] Per-user checkout rate limited
- [ ] Per-project concurrent session limit
- [ ] S3 request rate considered

## 9. Network Security

### 9.1 TLS Configuration
- [ ] S3 connections use HTTPS
- [ ] Redis connections use TLS in production
- [ ] Vault connections use TLS

### 9.2 Network Isolation
- [ ] Workspace directories not exposed via network shares
- [ ] S3 bucket not publicly accessible

## 10. Periodic Security Tasks

### 10.1 Weekly
- [ ] Review failed validation logs
- [ ] Check for orphaned workspace directories
- [ ] Verify backup integrity

### 10.2 Monthly
- [ ] Rotate encryption keys (if not automated)
- [ ] Review access logs for anomalies
- [ ] Update dependencies for security patches

### 10.3 Quarterly
- [ ] Penetration test workspace isolation
- [ ] Review and update this checklist
- [ ] Security training for development team

## 11. Compliance Considerations

### 11.1 Data Retention
- [ ] Workspace data retention policy documented
- [ ] Automatic cleanup of old sessions
- [ ] Audit logs retained per policy

### 11.2 Data Classification
- [ ] Project data classification documented
- [ ] Encryption requirements per classification
- [ ] Access controls per classification

## Security Test Commands

### Run All Security-Related Tests
```bash
# Path traversal tests
go test -v -run "Traversal|Validate" ./internal/workspaces/

# Race condition detection
go test -race ./internal/workspaces/

# Concurrent access tests
go test -v -run "Concurrent|Conflict" ./internal/workspaces/

# Full integration suite
go test -v -run "Integration" ./internal/workspaces/
```

### Verify File Permissions
```bash
# Check workspace sandbox permissions
find /path/to/sandboxes -type d -exec stat -c '%a %n' {} \;
# Should show 755 or 700 for directories

# Check key file permissions
stat -c '%a' /path/to/encryption/key
# Should show 600
```

### Check for Sensitive Data Exposure
```bash
# Search for potential credential exposure in code
grep -rn "password\|secret\|key\|token" internal/ --include="*.go" | grep -v "_test.go"

# Verify no hardcoded credentials
grep -rn "AKIA\|aws_secret" internal/
```

## Incident Response

If a security issue is discovered:

1. **Immediate**: Disable affected functionality if critical
2. **Within 1 hour**: Document the issue in security tracker
3. **Within 4 hours**: Assess impact and develop fix
4. **Within 24 hours**: Deploy fix to production
5. **Within 48 hours**: Post-incident review

## Sign-Off

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Security Lead | | | |
| Engineering Lead | | | |
| DevOps Lead | | | |

---

*Last Updated: 2025-01*
*Next Review: 2025-04*
