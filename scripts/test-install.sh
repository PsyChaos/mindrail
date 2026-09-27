#!/bin/sh
# Exercise the install boundary with real filesystem objects. Linux/coreutils is
# intentional: linux/amd64 is the verified native installation target.
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/mindrail-install-test.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT HUP INT TERM

cache=$scratch/go-cache
outside=$scratch/outside
mkdir -p "$cache" "$outside"
printf '%s\n' 'outside-must-not-change' >"$outside/sentinel"
outside_hash=$(sha256sum "$outside/sentinel")

run_install() {
	GOCACHE=$cache make -s -C "$repo" install "DESTDIR=$1" "BINDIR=$2"
}

expect_refused() {
	name=$1
	destdir=$2
	bindir=$3
	if run_install "$destdir" "$bindir" >"$scratch/$name.out" 2>"$scratch/$name.err"; then
		echo "install-test: $name unexpectedly succeeded" >&2
		exit 1
	fi
	if find "$scratch" -name '.mindrail.install.*' -print -quit | grep -q .; then
		echo "install-test: $name left a temporary install file" >&2
		exit 1
	fi
}

# Control: an absent destination is installed as one regular executable.
control=$scratch/control
run_install "$control" /usr/bin
test -f "$control/usr/bin/mindrail"
test -x "$control/usr/bin/mindrail"
test ! -L "$control/usr/bin/mindrail"
version_json=$("$control/usr/bin/mindrail" version --json)
printf '%s\n' "$version_json" | grep -q '"version":"0.2.0"'

# Replacing an existing regular file remains atomic and leaves no temp file.
replacement=$scratch/replacement
mkdir -p "$replacement/usr/bin"
printf '%s\n' old-binary >"$replacement/usr/bin/mindrail"
old_hash=$(sha256sum "$replacement/usr/bin/mindrail")
run_install "$replacement" /usr/bin
test -f "$replacement/usr/bin/mindrail"
test -x "$replacement/usr/bin/mindrail"
test "$(sha256sum "$replacement/usr/bin/mindrail")" != "$old_hash"

# A directory at the final destination must not absorb the temporary binary.
directory_target=$scratch/directory-target
mkdir -p "$directory_target/usr/bin/mindrail"
expect_refused destination-directory "$directory_target" /usr/bin
test -d "$directory_target/usr/bin/mindrail"
test -z "$(find "$directory_target/usr/bin/mindrail" -mindepth 1 -print -quit)"

# A final symlink is refused and its outside target remains byte-identical.
symlink_target=$scratch/symlink-target
mkdir -p "$symlink_target/usr/bin"
ln -s "$outside/sentinel" "$symlink_target/usr/bin/mindrail"
expect_refused destination-symlink "$symlink_target" /usr/bin
test -L "$symlink_target/usr/bin/mindrail"
test "$(sha256sum "$outside/sentinel")" = "$outside_hash"

# A parent symlink that resolves outside DESTDIR is rejected before any write.
parent_link=$scratch/parent-link
mkdir -p "$parent_link"
ln -s "$outside" "$parent_link/usr"
expect_refused parent-symlink-escape "$parent_link" /usr/bin
test ! -e "$outside/bin"
test "$(sha256sum "$outside/sentinel")" = "$outside_hash"

# Lexical traversal is rejected after canonical resolution.
traversal_root=$scratch/traversal/root
mkdir -p "$traversal_root" "$scratch/traversal-outside"
expect_refused traversal "$traversal_root" /../../traversal-outside/bin
test ! -e "$scratch/traversal-outside/bin"

# DESTDIR=/ contains every absolute resolved target. Exercise that boundary
# against a scratch destination directory so this test cannot write /usr/bin;
# the later destination-shape guard must be the reason for refusal.
root_boundary=$scratch/root-boundary
mkdir -p "$root_boundary/mindrail"
expect_refused destdir-root / "$root_boundary"
grep -q 'destination must be absent or a regular file' "$scratch/destdir-root.err"
if grep -q 'escapes DESTDIR' "$scratch/destdir-root.err"; then
	echo "install-test: DESTDIR=/ incorrectly rejected an absolute target" >&2
	exit 1
fi
test -d "$root_boundary/mindrail"
test -z "$(find "$root_boundary/mindrail" -mindepth 1 -print -quit)"

test "$(sha256sum "$outside/sentinel")" = "$outside_hash"
test -z "$(find "$scratch" -name '.mindrail.install.*' -print -quit)"
echo "install-test: PASS"
