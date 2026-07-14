#!/usr/bin/env sh
# Acthur Installer — https://acthur.dev
# Usage: curl -fsSL https://install.acthur.dev | sh
#
# This script:
#   1. Detects your OS and architecture
#   2. Downloads the correct binary from GitHub Releases
#   3. Installs it to ~/.acthur/bin
#   4. Adds ~/.acthur/bin to your PATH
#   5. Runs `acthur doctor` to verify the installation

set -e

ACTHUR_REPO="acthurhq/acthur"
ACTHUR_BIN_DIR="${HOME}/.acthur/bin"
ACTHUR_BIN="${ACTHUR_BIN_DIR}/acthur"
GITHUB_API="https://api.github.com/repos/${ACTHUR_REPO}/releases/latest"

# ── Colours ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
RESET='\033[0m'

info()    { printf "${CYAN}  →  ${RESET}%s\n" "$1"; }
success() { printf "${GREEN}  ✓  ${RESET}%s\n" "$1"; }
error()   { printf "${RED}  ✗  ${RESET}%s\n" "$1" >&2; exit 1; }
banner()  { printf "\n${BOLD}  ✦  Acthur Installer${RESET}\n${DIM}     https://acthur.dev${RESET}\n\n"; }

# ── Detect OS and architecture ───────────────────────────────────────────────
detect_platform() {
  OS="$(uname -s)"
  ARCH="$(uname -m)"

  case "${OS}" in
    Linux)  OS="linux" ;;
    Darwin) OS="darwin" ;;
    *)      error "Unsupported OS: ${OS}. Please install manually from https://github.com/${ACTHUR_REPO}/releases" ;;
  esac

  case "${ARCH}" in
    x86_64 | amd64) ARCH="amd64" ;;
    arm64 | aarch64) ARCH="arm64" ;;
    *) error "Unsupported architecture: ${ARCH}. Please install manually from https://github.com/${ACTHUR_REPO}/releases" ;;
  esac

  PLATFORM="${OS}_${ARCH}"
}

# ── Get latest release version ───────────────────────────────────────────────
get_latest_version() {
  if command -v curl >/dev/null 2>&1; then
    VERSION="$(curl -fsSL "${GITHUB_API}" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  elif command -v wget >/dev/null 2>&1; then
    VERSION="$(wget -qO- "${GITHUB_API}" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  else
    error "curl or wget is required to install Acthur"
  fi

  if [ -z "${VERSION}" ]; then
    error "Failed to determine latest Acthur version. Check your internet connection."
  fi
}

# ── Download binary ──────────────────────────────────────────────────────────
download_binary() {
  ARCHIVE_NAME="acthur_${VERSION#v}_${PLATFORM}.tar.gz"
  DOWNLOAD_URL="https://github.com/${ACTHUR_REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"
  CHECKSUMS_NAME="acthur_${VERSION#v}_checksums.txt"
  CHECKSUMS_URL="https://github.com/${ACTHUR_REPO}/releases/download/${VERSION}/${CHECKSUMS_NAME}"

  info "Downloading Acthur ${VERSION} for ${PLATFORM}..."

  TMP_DIR="$(mktemp -d)"
  ARCHIVE_PATH="${TMP_DIR}/${ARCHIVE_NAME}"
  CHECKSUMS_PATH="${TMP_DIR}/${CHECKSUMS_NAME}"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --progress-bar "${DOWNLOAD_URL}" -o "${ARCHIVE_PATH}" || \
      error "Download failed from ${DOWNLOAD_URL}"
    curl -fsSL "${CHECKSUMS_URL}" -o "${CHECKSUMS_PATH}" || \
      error "Checksum manifest download failed from ${CHECKSUMS_URL}"
  else
    wget -q --show-progress "${DOWNLOAD_URL}" -O "${ARCHIVE_PATH}" || \
      error "Download failed from ${DOWNLOAD_URL}"
    wget -q "${CHECKSUMS_URL}" -O "${CHECKSUMS_PATH}" || \
      error "Checksum manifest download failed from ${CHECKSUMS_URL}"
  fi

  EXPECTED_SHA256="$(awk -v archive="${ARCHIVE_NAME}" '$2 == archive { print $1; exit }' "${CHECKSUMS_PATH}")"
  [ -n "${EXPECTED_SHA256}" ] || error "Checksum manifest has no entry for ${ARCHIVE_NAME}"

  if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL_SHA256="$(sha256sum "${ARCHIVE_PATH}" | awk '{ print $1 }')"
  elif command -v shasum >/dev/null 2>&1; then
    ACTUAL_SHA256="$(shasum -a 256 "${ARCHIVE_PATH}" | awk '{ print $1 }')"
  else
    error "sha256sum or shasum is required to verify the Acthur release"
  fi

  [ "${ACTUAL_SHA256}" = "${EXPECTED_SHA256}" ] || \
    error "Checksum verification failed for ${ARCHIVE_NAME}"

  # Extract
  tar -xzf "${ARCHIVE_PATH}" -C "${TMP_DIR}"

  # Install
  mkdir -p "${ACTHUR_BIN_DIR}"
  mv "${TMP_DIR}/acthur" "${ACTHUR_BIN}"
  chmod +x "${ACTHUR_BIN}"

  rm -rf "${TMP_DIR}"
}

# ── Add to PATH ───────────────────────────────────────────────────────────────
add_to_path() {
  # Detect shell
  SHELL_NAME="$(basename "${SHELL:-/bin/sh}")"
  PROFILE=""

  case "${SHELL_NAME}" in
    bash)
      PROFILE="${HOME}/.bashrc"
      [ -f "${HOME}/.bash_profile" ] && PROFILE="${HOME}/.bash_profile"
      ;;
    zsh)  PROFILE="${HOME}/.zshrc" ;;
    fish) PROFILE="${HOME}/.config/fish/config.fish" ;;
    *)    PROFILE="${HOME}/.profile" ;;
  esac

  PATH_LINE='export PATH="${HOME}/.acthur/bin:${PATH}"'
  FISH_LINE='set -gx PATH "$HOME/.acthur/bin" $PATH'

  # Check if already in PATH
  case ":${PATH}:" in
    *":${ACTHUR_BIN_DIR}:"*)
      return 0 ;;
  esac

  # Add to profile
  if [ -n "${PROFILE}" ]; then
    if [ "${SHELL_NAME}" = "fish" ]; then
      echo "" >> "${PROFILE}"
      echo "# Acthur" >> "${PROFILE}"
      echo "${FISH_LINE}" >> "${PROFILE}"
    else
      echo "" >> "${PROFILE}"
      echo "# Acthur" >> "${PROFILE}"
      echo "${PATH_LINE}" >> "${PROFILE}"
    fi
    info "Added Acthur to PATH in ${PROFILE}"
  fi

  # Also export for the current session
  export PATH="${ACTHUR_BIN_DIR}:${PATH}"
}

# ── Verify installation ───────────────────────────────────────────────────────
verify() {
  VERSION_OUTPUT="$("${ACTHUR_BIN}" version 2>&1)" || \
    error "Installation verification failed. Binary at ${ACTHUR_BIN} is not executable."

  EXPECTED_VERSION="${VERSION#v}"
  printf '%s\n' "${VERSION_OUTPUT}" | grep -F "Version:" | grep -F "${EXPECTED_VERSION}" >/dev/null 2>&1 || \
    error "Installation verification failed. Binary does not report requested version ${VERSION}."

  success "Acthur ${VERSION} installed successfully"
}

# ── Main ──────────────────────────────────────────────────────────────────────
main() {
  banner
  detect_platform
  get_latest_version
  download_binary
  add_to_path
  verify

  printf "\n"
  printf "  ${BOLD}Next steps:${RESET}\n\n"
  printf "  ${DIM}Restart your shell or run:${RESET}\n"
  printf "    source ~/.zshrc  ${DIM}(or .bashrc / .profile)${RESET}\n\n"
  printf "  ${DIM}Then get started:${RESET}\n"
  printf "    acthur doctor           ${DIM}check your environment${RESET}\n"
  printf "    acthur new my-project   ${DIM}create a new project${RESET}\n"
  printf "    acthur --help           ${DIM}see all commands${RESET}\n"
  printf "\n"
  printf "  ${DIM}Docs: https://acthur.dev${RESET}\n\n"
}

main
