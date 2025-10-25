# Security

manifold implements several security measures to ensure safe execution of agent workflows.

## Execution Security

### Locked WORKDIR
- All tool execution is restricted to a configurable working directory
- No access to files outside the WORKDIR
- Prevents directory traversal attacks

### No Shell Execution
- The `run_cli` tool executes binaries directly, not through shell
- Prevents shell injection attacks
- Only bare binary names are accepted (no paths)

### Binary Blocking
Configure blocked binaries via environment variable:

```env
BLOCK_BINARIES=rm,sudo,su,chmod,chown
```

## Runtime Safety

### Timeouts
Configure execution timeouts to prevent runaway processes:

```env
MAX_COMMAND_SECONDS=30           # Tool execution timeout
AGENT_RUN_TIMEOUT_SECONDS=300    # Overall agent timeout
STREAM_RUN_TIMEOUT_SECONDS=60    # Stream timeout
```

### Output Truncation
Prevent memory exhaustion from large outputs:

```env
OUTPUT_TRUNCATE_BYTES=65536      # Truncate outputs larger than 64KB
```

## Authentication and Authorization

### OIDC / OAuth2 Integration
manifold supports OpenID Connect (default) and configurable OAuth2 providers for authentication:

```yaml
auth:
  enabled: true
  provider: oidc # or oauth2
  issuerURL: "https://your-oidc-provider.com"
  clientID: "your-client-id"
  clientSecret: "your-client-secret"
  redirectURL: "http://localhost:32180/auth/callback"
  oauth2:
    authURL: ""
    tokenURL: ""
    userInfoURL: ""
    scopes: []
```

### Session Management
- Secure cookie-based sessions
- Configurable session timeout
- CSRF protection

## API Security

### Input Validation
- All inputs are validated and sanitized
- JSON schema validation for API requests
- Parameter length limits

### Rate Limiting
Built-in rate limiting to prevent abuse:
- Per-IP rate limits
- Per-user rate limits (when authenticated)
- Configurable windows and thresholds

## Best Practices

1. **Environment Isolation**: Run manifold in containers or isolated environments
2. **Principle of Least Privilege**: Use minimal required permissions
3. **Regular Updates**: Keep dependencies up to date
4. **Monitoring**: Enable comprehensive logging and monitoring
5. **Network Security**: Use HTTPS in production, restrict network access
