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

## Parallel web search and fetch

[Parallel Search MCP](https://docs.parallel.ai/integrations/mcp/search-mcp) provides
free, anonymous web search and page extraction over Streamable HTTP. No Parallel
API key or OAuth login is required. The free endpoint has rate limits and is
intended for exploration and light use.

The [standalone example](examples/mcp-parallel.yaml) can be selected when launching
an existing Manifold installation from the repository root:

```bash
MCP_CONFIG="$PWD/docs/examples/mcp-parallel.yaml" ./dist/manifold
```

For local binary installation instructions, see [the deployment guide](deployment.md).
`MCP_CONFIG` selects an external MCP file, whose root is `servers:` rather than
`mcp:`. Its server list replaces the MCP servers from `config.yaml`. To retain
other servers, merge the `parallel` entry into your existing `mcp.yaml` instead.

Manifold registers the discovered tools as `parallel_web_search` and
`parallel_web_fetch`. Set `enableTools: true` for the
caller. If your top-level tools or specialist has an `allowTools` list, include
these names there to expose them. Search requires `objective` and
`search_queries`; fetch requires `urls`. For example, tool arguments are:

```json
{"objective":"Find the Go documentation for context cancellation", "search_queries":["Go context cancellation documentation"]}
```

```json
{"urls":["https://go.dev/blog/context"], "objective":"Explain context cancellation"}
```

The example sends a Manifold User-Agent and uses the existing 30-second HTTP
request timeout. It does not enable Parallel in the default configuration or
change the selected LLM provider.

## Supported MCP Features

- Tool registration from external servers
- Automatic tool discovery and registration
- Environment variable passing to stdio servers
- Process lifecycle management (stdio)
- Remote servers over Streamable HTTP transport

## Deployment Notes

- If you use Docker-based stdio servers, make sure the `manifold` container can reach the Docker socket. The included compose file already mounts `/var/run/docker.sock` into the `manifold` service.
- Path-dependent MCP servers rely on the active project workspace path under `WORKDIR`.
- Project skills load from `skills/` inside the active project root. Universal read-only skills load from `$HOME/.manifold/skills` and `$HOME/.agents/skills` through dedicated skill tools, without widening normal file tool access.
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
