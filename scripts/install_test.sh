#!/usr/bin/env sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

new_fixture() {
  FIXTURE_DIR=$(mktemp -d)
  mkdir -p "$FIXTURE_DIR/bin" "$FIXTURE_DIR/home"
  printf 'released archive fixture' >"$FIXTURE_DIR/archive"

  cat >"$FIXTURE_DIR/bin/uname" <<'EOF'
#!/usr/bin/env sh
case "$1" in
  -s) printf 'Linux\n' ;;
  -m) printf 'x86_64\n' ;;
esac
EOF
  cat >"$FIXTURE_DIR/bin/curl" <<'EOF'
#!/usr/bin/env sh
url=''
output=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) output=$2; shift 2 ;;
    http*) url=$1; shift ;;
    *) shift ;;
  esac
done
case "$url" in
  */releases/latest) printf '{"tag_name":"v1.2.3"}\n' ;;
  */acthur_1.2.3_linux_amd64.tar.gz) cp "$TEST_ARCHIVE" "$output" ;;
  */acthur_1.2.3_checksums.txt) cp "$TEST_CHECKSUMS" "$output" ;;
  *) exit 22 ;;
esac
EOF
  cat >"$FIXTURE_DIR/bin/tar" <<'EOF'
#!/usr/bin/env sh
: >"$TEST_EXTRACT_MARKER"
destination=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    -C) destination=$2; shift 2 ;;
    *) shift ;;
  esac
done
cat >"$destination/acthur" <<'BIN'
#!/usr/bin/env sh
printf '  Version:  v1.2.3\n'
BIN
chmod +x "$destination/acthur"
EOF
  chmod +x "$FIXTURE_DIR/bin/uname" "$FIXTURE_DIR/bin/curl" "$FIXTURE_DIR/bin/tar"
}

run_installer() {
  PATH="$FIXTURE_DIR/bin:$PATH" HOME="$FIXTURE_DIR/home" SHELL=/bin/sh \
    TEST_ARCHIVE="$FIXTURE_DIR/archive" TEST_CHECKSUMS="$FIXTURE_DIR/checksums" \
    TEST_EXTRACT_MARKER="$FIXTURE_DIR/extracted" \
    sh "$ROOT_DIR/scripts/install.sh" >"$FIXTURE_DIR/output" 2>&1
}

test_checksum_mismatch_stops_before_extraction() {
  new_fixture
  printf '%064d  acthur_1.2.3_linux_amd64.tar.gz\n' 0 >"$FIXTURE_DIR/checksums"

  run_installer && fail "installer accepted an archive whose SHA-256 did not match the release manifest"
  [ ! -e "$FIXTURE_DIR/extracted" ] || fail "installer extracted an archive before verifying its checksum"
  grep -qi 'checksum verification failed' "$FIXTURE_DIR/output" || fail "installer did not explain the checksum mismatch"
  rm -rf "$FIXTURE_DIR"
}

test_missing_checksum_entry_stops_before_extraction() {
  new_fixture
  printf '%064d  another_archive.tar.gz\n' 0 >"$FIXTURE_DIR/checksums"

  run_installer && fail "installer accepted a manifest without its archive"
  [ ! -e "$FIXTURE_DIR/extracted" ] || fail "installer extracted an archive missing from the manifest"
  grep -qi 'no entry' "$FIXTURE_DIR/output" || fail "installer did not identify the missing manifest entry"
  rm -rf "$FIXTURE_DIR"
}

test_valid_checksum_installs_requested_version() {
  new_fixture
  archive_sha256=$(sha256sum "$FIXTURE_DIR/archive" | awk '{ print $1 }')
  printf '%s  acthur_1.2.3_linux_amd64.tar.gz\n' "$archive_sha256" >"$FIXTURE_DIR/checksums"

  run_installer || { sed -n '1,120p' "$FIXTURE_DIR/output" >&2; fail "installer rejected a valid release archive"; }
  [ -e "$FIXTURE_DIR/extracted" ] || fail "installer did not extract the verified archive"
  "$FIXTURE_DIR/home/.acthur/bin/acthur" version | grep -q 'Version:  v1.2.3' || \
    fail "installed binary did not report the requested release version"
  rm -rf "$FIXTURE_DIR"
}

test_checksum_mismatch_stops_before_extraction
test_missing_checksum_entry_stops_before_extraction
test_valid_checksum_installs_requested_version
printf 'PASS: install.sh verifies release integrity before installation\n'
