#!/bin/sh
# Test committed source without exposing host paths, credentials or Docker socket.
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
test "$#" -le 1
source_ref=${1:-HEAD}
if [ "$source_ref" = --worktree ]; then source_ref=HEAD; fi
revision=$(git rev-parse --verify "$source_ref^{commit}")
mkdir -p .cache
probe_root=$(mktemp -d "$PWD/.cache/container-probe.XXXXXXXX")
context="$probe_root/context"
mkdir -p "$context"
if [ "${1:-}" = --worktree ]; then
  git ls-files --cached --others --exclude-standard -z | tar --null -T - -cf "$probe_root/source.tar"
  git status --porcelain=v1 > "$probe_root/source-status"
  git diff --binary HEAD > "$probe_root/source.patch"
else
  git archive "$revision" > "$probe_root/source.tar"
fi
tar -xf "$probe_root/source.tar" -C "$context"
# Explicitly include the current harness, including its first pre-commit run.
cp tests/container/Dockerfile "$context/Dockerfile"
cp scripts/container-check.sh "$context/scripts/container-check.sh"
printf '%s\n' "$revision" > "$probe_root/source-revision"
shasum -a 256 "$probe_root/source.tar" "$context/Dockerfile" \
  "$context/scripts/container-check.sh" > "$probe_root/inputs.sha256"
printf 'Container evidence directory: %s\n' "$probe_root"
docker build --platform=linux/arm64 --network=none --iidfile "$probe_root/image-id" "$context"
image_id=$(cat "$probe_root/image-id")
status=0
docker run --rm --platform=linux/arm64 --read-only --network=none \
  --cap-drop=ALL --security-opt=no-new-privileges --pids-limit=256 \
  --memory=4g --cpus=2 --tmpfs=/tmp:rw,exec,nosuid,nodev,size=2g \
  --tmpfs=/full:rw,noexec,nosuid,nodev,size=64k,mode=0700,uid=10001,gid=10001 \
  --env BREWWARDEN_TEST_FULL_DISK=/full \
  "$image_id" > "$probe_root/check.stdout" 2> "$probe_root/check.stderr" || status=$?
printf '%s\n' "$status" > "$probe_root/check.status"
cat "$probe_root/check.stdout"
cat "$probe_root/check.stderr" >&2
exit "$status"
