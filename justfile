# The platform of this machine, as mygo names it.
host := `case "$(uname -s)-$(uname -m)" in Darwin-arm64) echo darwin-arm64;; Darwin-x86_64) echo darwin-amd64;; Linux-x86_64) echo linux-amd64;; Linux-aarch64|Linux-arm64) echo linux-arm64;; *) echo unsupported;; esac`

[private]
default:
    @just --list

# Run the app with live reload.
dev:
    go tool mygo dev

# Vet everything.
check:
    go vet ./...

# Build this machine's production app into build/.
build platform=host:
    go tool mygo build -platform {{replace(platform, "-", "/")}}

# Build and open this machine's production app.
run: (build host)
    #!/usr/bin/env bash
    set -euo pipefail
    "build/{{host}}/kupic"

# Render the app's pages to PNG files, for looking at them.
snapshots dir="/tmp/kupic-shots":
    mkdir -p {{dir}}
    KUPIC_SNAPSHOTS={{dir}} go test -run TestSnapshots .

# Remove build output.
clean:
    rm -rf build .mygo
