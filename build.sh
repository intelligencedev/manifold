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

echo "Build complete. Binaries are available in the ./dist directory"