#!/bin/bash
set -e

echo "Building Manifold MCP Server..."
cd "$(dirname "$0")"
go build -o mcp-manifold .

echo "Build successful!"
echo "Usage: ./mcp-manifold"