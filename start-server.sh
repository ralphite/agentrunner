#!/usr/bin/env bash

set -euo pipefail

# Ensure we run from the project root
cd "$(dirname "$0")"

# Prepend Homebrew's binary path on macOS to avoid architecture mismatches
# with Rosetta-based NVM installations (UX-04 / Mac arm64 native)
if [ -d "/opt/homebrew/bin" ]; then
    export PATH="/opt/homebrew/bin:$PATH"
fi

# Print usage information
show_usage() {
    echo "Usage: ./start-server.sh [options] [-- [arwebui_flags]]"
    echo ""
    echo "Options:"
    echo "  -h, --help        Show this help message and exit"
    echo "  --skip-build      Skip building/compiling and start the server immediately"
    echo ""
    echo "arwebui flags (can be passed after '--' or as normal flags if they don't conflict):"
    echo "  -addr <addr>      Address to listen on (default: 127.0.0.1:8788)"
    echo "  -env-file <file>  Load environment variables from file (e.g. GEMINI_API_KEY)"
    echo "  -no-daemon        Do not spawn 'ar daemon' (use an external daemon)"
    echo "  -runtime <dir>    Scratch/runtime directory (default: runtime)"
    echo ""
    echo "Examples:"
    echo "  ./start-server.sh"
    echo "  ./start-server.sh --skip-build"
    echo "  ./start-server.sh -- -addr 127.0.0.1:9000 -env-file .env"
}

SKIP_BUILD=false
SERVER_ARGS=()

# Parse script arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            show_usage
            exit 0
            ;;
        --skip-build)
            SKIP_BUILD=true
            shift
            ;;
        --)
            shift
            SERVER_ARGS+=("$@")
            break
            ;;
        *)
            SERVER_ARGS+=("$1")
            shift
            ;;
    esac
done

if [ "$SKIP_BUILD" = false ]; then
    echo "==== Checking Dependencies ===="
    if ! command -v go >/dev/null 2>&1; then
        echo "Error: 'go' is not installed or not in PATH." >&2
        exit 1
    fi
    if ! command -v node >/dev/null 2>&1; then
        echo "Error: 'node' is not installed or not in PATH." >&2
        exit 1
    fi
    if ! command -v npm >/dev/null 2>&1; then
        echo "Error: 'npm' is not installed or not in PATH." >&2
        exit 1
    fi

    echo "Go version: $(go version)"
    echo "Node version: $(node --version)"

    echo ""
    echo "==== 1. Building Frontend React/Vite App ===="
    (
        cd webui/frontend
        if [ ! -d node_modules ]; then
            echo "Installing frontend dependencies..."
            npm ci || npm install
        fi
        echo "Compiling frontend assets..."
        npm run build
    )

    echo ""
    echo "==== 2. Compiling Go Binaries ===="
    mkdir -p bin

    echo "Compiling CLI (ar)..."
    go build -buildvcs=false -o bin/ar ./cmd/agentrunner

    echo "Compiling Web UI Server (arwebui)..."
    go build -buildvcs=false -o bin/arwebui ./webui

    echo "Build completed successfully!"
    echo ""
fi

# Ensure compiled binaries exist when skipping build
if [ ! -f "bin/ar" ] || [ ! -f "bin/arwebui" ]; then
    echo "Error: Compiled binaries not found in ./bin/." >&2
    echo "Please run the script without --skip-build first to compile them." >&2
    exit 1
fi

echo "==== Starting Web UI Server ===="
echo "Command: ./bin/arwebui -ar ./bin/ar ${SERVER_ARGS[*]:-}"
echo "--------------------------------------------------------"
exec ./bin/arwebui -ar ./bin/ar "${SERVER_ARGS[@]}"
