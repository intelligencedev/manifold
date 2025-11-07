# Git Hooks

This directory contains Git hooks for the Manifold project.

## Pre-commit Hook

The pre-commit hook runs automatically before each commit to ensure code quality. It performs the following checks:

### Checks Performed

1. **Code Formatting** (`gofmt`)
   - Ensures all Go files are properly formatted
   - Fix: `gofmt -w <file>` or `make fmt`

2. **Static Analysis** (`go vet`)
   - Detects common Go programming errors
   - Fix issues manually based on output

3. **Unused Code Detection** (`staticcheck`)
   - Identifies unused variables, functions, imports, etc.
   - Requires: `go install honnef.co/go/tools/cmd/staticcheck@latest`

4. **Build Validation** (`go build`)
   - Verifies that changed packages compile successfully
   - Prevents broken code from being committed

### Installation

Install the hooks using the installation script:

```bash
./scripts/install-hooks.sh
```

Or manually copy the hook:

```bash
cp scripts/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

### Bypassing the Hook

In exceptional cases, you can skip the pre-commit hook:

```bash
git commit --no-verify
```

**Note:** Only use `--no-verify` when absolutely necessary, as it bypasses all code quality checks.

### Troubleshooting

#### staticcheck not installed

If you see a warning about staticcheck not being installed:

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest
```

#### Hook fails on formatted code

Ensure your editor is configured to run `gofmt` on save, or run:

```bash
make fmt
# or
gofmt -w .
```

#### False positives in whisper.cpp

The hook filters out errors from the vendored `whisper.cpp` dependency. If you see issues with this library, they can be safely ignored.

## Customization

To customize the checks, edit `scripts/pre-commit` and run `./scripts/install-hooks.sh` again.

### Available Options

- **Skip certain checks**: Comment out sections in the script
- **Add new checks**: Add additional linters or tests
- **Change severity**: Convert checks to warnings instead of errors
