#!/bin/bash

# Create dist directory if it doesn't exist
mkdir -p ./dist

echo "Building frontend..."
cd frontend
nvm use 20
npm run build
cd ..

echo "Building main manifold application..."
go build -ldflags="-s -w" -trimpath -o ./dist/manifold .

echo "Building mcp-manifold application..."
go build -ldflags="-s -w" -trimpath -o ./dist/mcp-manifold ./cmd/mcp-manifold

echo "Building mcp-recfut-asi application..."
go build -ldflags="-s -w" -trimpath -o ./dist/mcp-recfut-asi ./cmd/mcp-recfut-asi

echo "Build complete. Binaries are available in the ./dist directory"