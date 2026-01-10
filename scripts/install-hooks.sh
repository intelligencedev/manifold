#!/bin/bash
#
# Install git hooks for the Manifold project
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOOKS_DIR="$REPO_ROOT/.git/hooks"

echo "Installing git hooks for Manifold..."

# Check if we're in a git repository
if [ ! -d "$REPO_ROOT/.git" ]; then
    echo "Error: Not in a git repository"
    exit 1
fi

# Install pre-commit hook
if [ -f "$SCRIPT_DIR/pre-commit" ]; then
    cp "$SCRIPT_DIR/pre-commit" "$HOOKS_DIR/pre-commit"
    chmod +x "$HOOKS_DIR/pre-commit"
    echo "✓ Installed pre-commit hook"
else
    echo "Error: pre-commit script not found at $SCRIPT_DIR/pre-commit"
    exit 1
fi

echo ""
echo "✓ Git hooks installed successfully!"
echo ""
echo "The pre-commit hook will now run automatically before each commit to check:"
echo "  • Code formatting (gofmt)"
echo "  • Static analysis (go vet)"
echo "  • Unused code detection (staticcheck)"
echo "  • Build validation"
echo ""
echo "To skip the hook in exceptional cases, use: git commit --no-verify"
echo ""

# Check if staticcheck is installed
if ! command -v staticcheck >/dev/null 2>&1; then
    echo "⚠️  staticcheck is not installed. Install it for unused code detection:"
    echo "   go install honnef.co/go/tools/cmd/staticcheck@latest"
    echo ""
fi
