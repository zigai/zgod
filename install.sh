#!/bin/sh
set -e

REPO="zigai/zgod"
BINARY="zgod"

get_os() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        linux) echo "linux" ;;
        darwin) echo "darwin" ;;
        *) echo "Unsupported OS: $os" >&2; exit 1 ;;
    esac
}

get_arch() {
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
    esac
}

get_latest_version() {
    curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
}

main() {
    os=$(get_os)
    arch=$(get_arch)

    if [ -n "$VERSION" ]; then
        version="$VERSION"
    else
        echo "Fetching latest version..."
        version=$(get_latest_version)
    fi

    if [ -z "$version" ]; then
        echo "Failed to determine version" >&2
        exit 1
    fi

    version_num="${version#v}"
    archive="${BINARY}_${version_num}_${os}_${arch}.tar.gz"
    url="https://github.com/${REPO}/releases/download/${version}/${archive}"
    checksum_url="https://github.com/${REPO}/releases/download/${version}/checksums.txt"

    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    echo "Downloading ${archive}..."
    curl -fsSL "$url" -o "$tmpdir/$archive"

    echo "Downloading checksums..."
    curl -fsSL "$checksum_url" -o "$tmpdir/checksums.txt"

    echo "Verifying checksum..."
    cd "$tmpdir"
    expected=$(grep -F "$archive" checksums.txt | awk '{print $1}')
    if [ -z "$expected" ]; then
        echo "Checksum not found for $archive" >&2
        exit 1
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$archive" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$archive" | awk '{print $1}')
    else
        echo "No SHA-256 tool found" >&2
        exit 1
    fi

    if [ "$expected" != "$actual" ]; then
        echo "Checksum mismatch!" >&2
        echo "Expected: $expected" >&2
        echo "Actual:   $actual" >&2
        exit 1
    fi
    echo "Checksum verified."

    echo "Extracting..."
    tar -xzf "$archive"

    if [ -w /usr/local/bin ]; then
        install_dir="/usr/local/bin"
    else
        install_dir="$HOME/.local/bin"
        mkdir -p "$install_dir"
    fi

    echo "Installing to $install_dir..."
    mv "$BINARY" "$install_dir/"
    chmod +x "$install_dir/$BINARY"

    echo ""
    echo "Successfully installed $BINARY $version to $install_dir/$BINARY"

    if ! echo "$PATH" | tr ':' '\n' | grep -qx "$install_dir"; then
        echo ""
        echo "NOTE: $install_dir is not in your PATH."
        echo "Add it with: export PATH=\"\$PATH:$install_dir\""
    fi
}

main
