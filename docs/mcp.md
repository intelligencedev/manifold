# MCP Client

manifold includes a Model Context Protocol (MCP) client to register external server tools. This allows you to extend manifold's capabilities by connecting to external tool providers.

## Configuration

Configure MCP servers in your `config.yaml`:

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

## Supported MCP Features

- Tool registration from external servers
- Automatic tool discovery and registration
- Environment variable passing to servers
- Process lifecycle management

## Example MCP Servers

1. **Filesystem Server**: Provides file system operations
2. **Database Server**: Provides database query capabilities
3. **Web Server**: Provides web scraping and API access
4. **Custom Servers**: Build your own MCP-compatible servers

## Security Considerations

- MCP servers run as separate processes
- Environment variables are isolated per server
- Tool execution follows the same security model as built-in tools
- Review server code before adding to production environments