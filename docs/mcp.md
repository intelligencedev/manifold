# MCP Client

Manifold includes a Model Context Protocol (MCP) client that can register external server tools. This lets you extend the orchestrator with stdio-based or remote HTTP MCP servers.

For a fresh clone, MCP is optional. The default deployment works without any extra MCP services.

## Configuration

Configure MCP servers in `config.yaml`. You can use either local stdio servers or remote HTTP servers.

Local stdio example:

```yaml
mcp:
  servers:
    - name: filesystem
      command: ["npx", "-y", "@modelcontextprotocol/server-filesystem", "/path/to/allowed/dir"]
    - name: database
      command: ["python", "-m", "mcp_server_postgres"]
      env:
        DATABASE_URL: "postgresql://user:pass@localhost/db"
```

Remote HTTP example:

```yaml
mcp:
  servers:
    - name: acme
      url: https://mcp.acme.com/mcp
      origin: https://manifold.local   # optional Origin header
      bearerToken: ${ACME_MCP_TOKEN}   # optional Authorization: Bearer
      headers:
        X-Client: Manifold
      http:
        timeoutSeconds: 30
        proxyURL: ""
        tls:
          insecureSkipVerify: false
          # caFile: /etc/ssl/certs/ca-bundle.crt
```

## Supported MCP Features

- Tool registration from external servers
- Automatic tool discovery and registration
- Environment variable passing to stdio servers
- Process lifecycle management (stdio)
- Remote servers over Streamable HTTP transport

## Deployment Notes

- If you use Docker-based stdio servers, make sure the `manifold` container can reach the Docker socket. The included compose file already mounts `/var/run/docker.sock` into the `manifold` service.
- Path-dependent MCP servers rely on the active project workspace path under `WORKDIR`.
- If auth is disabled, MCP servers are typically shared across the process.
- If auth is enabled and `pathDependent: true` is set, Manifold can instantiate per-user servers tied to the active project.

## Example MCP Servers

1. **Filesystem Server**: Provides file system operations
2. **Database Server**: Provides database query capabilities
3. **Web Server**: Provides web scraping and API access
4. **Custom Servers**: Build your own MCP-compatible servers

## Security Considerations

- MCP stdio servers run as separate processes
- Environment variables are isolated per stdio server
- Tool execution follows the same security model as built-in tools
- Review server code before adding to production environments
- For remote servers, only connect to trusted endpoints; set an Origin header and use TLS where possible.
