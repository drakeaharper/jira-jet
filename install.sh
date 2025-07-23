#!/bin/bash

# Install/Update script for jira-jet
# This script builds and installs the jet binary to make it available globally

set -e

echo "🚀 Installing/Updating jira-jet..."

# Check if we're in the correct directory
if [ ! -f "go.mod" ] || [ ! -f "main.go" ]; then
    echo "❌ Error: This script must be run from the jira-jet project directory"
    echo "   Make sure you're in the directory containing go.mod and main.go"
    exit 1
fi

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed or not in PATH"
    echo "   Please install Go from https://golang.org/dl/"
    exit 1
fi

# Get the current Go binary path
GO_BIN_PATH=$(go env GOPATH)/bin
if [ -z "$GO_BIN_PATH" ]; then
    GO_BIN_PATH="$HOME/go/bin"
fi

echo "📦 Building jet binary..."
go build -o jet .

echo "📋 Installing to $GO_BIN_PATH..."
# Create the bin directory if it doesn't exist
mkdir -p "$GO_BIN_PATH"

# Copy the binary to the Go bin directory
cp jet "$GO_BIN_PATH/jet"

# Make sure it's executable
chmod +x "$GO_BIN_PATH/jet"

echo "✅ Successfully installed jet to $GO_BIN_PATH"

# Check if GO_BIN_PATH is in PATH
if [[ ":$PATH:" != *":$GO_BIN_PATH:"* ]]; then
    echo ""
    echo "⚠️  Warning: $GO_BIN_PATH is not in your PATH"
    echo "   Add this line to your shell profile (.bashrc, .zshrc, etc.):"
    echo "   export PATH=\"\$PATH:$GO_BIN_PATH\""
    echo ""
    echo "   Or run: echo 'export PATH=\"\$PATH:$GO_BIN_PATH\"' >> ~/.bashrc"
    echo "   Then restart your terminal or run: source ~/.bashrc"
else
    echo "🎉 jet is now available globally!"
    echo ""
    echo "Try running: jet --help"
fi

# Test the installation
echo ""
echo "🧪 Testing installation..."
if "$GO_BIN_PATH/jet" --help > /dev/null 2>&1; then
    echo "✅ Installation test passed!"
else
    echo "❌ Installation test failed"
    exit 1
fi

echo ""
echo "🔄 To update jet in the future, just run this script again!"