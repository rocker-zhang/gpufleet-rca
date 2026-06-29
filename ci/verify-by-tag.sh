#!/usr/bin/env bash
# verify-by-tag.sh — prove the RELEASE consumption path: resolve the proto gen
# module by its PINNED TAG via the public module proxy, WITHOUT any local
# `replace` directive. rca consumes gpufleet-proto/gen/go as a published module,
# so this script confirms the pinned tag actually resolves the way an external
# consumer would AND guards against a stray `replace` creeping in during future
# development.
#
# It must be run from the module root. It operates on a throwaway COPY of the
# module (no working-tree mutation).
#
# The gen module is published as a Go submodule tag for the gen subdir, i.e.
# gen/go/v<VER> (Go derives the tag from the module subdir path; a module at
# .../gen/go is tagged gen/go/vX.Y.Z, not proto/vX.Y.Z). The module is public,
# so it resolves through the default proxy + checksum DB — no auth needed.
#
# Modes:
#   (default)            hard-fail if the by-tag path does not resolve.
#   VERIFY_BY_TAG_SOFT=1 still run the full attempt and print the exact go error,
#                        but exit 0 (used while an out-of-repo prerequisite — e.g.
#                        the proto tag not yet published — is unmet, so PR CI is
#                        not red-walled by an infra gap).
set -uo pipefail

GEN_MOD="github.com/rocker-zhang/gpufleet-proto/gen/go"
GEN_VER="$(go mod edit -json | python3 -c \
  'import json,sys;print(next(r["Version"] for r in json.load(sys.stdin)["Require"] if r["Path"]=="'"$GEN_MOD"'"))')"
echo ">> gen module require pin: ${GEN_MOD} ${GEN_VER}"
echo ">> expected proto submodule tag (Go convention): gen/go/${GEN_VER}"

# Resolve strictly through the public proxy so this mirrors an external
# consumer's release path; -mod=mod lets `go mod edit -dropreplace` take effect.
export GOFLAGS='-mod=mod'

SOFT="${VERIFY_BY_TAG_SOFT:-0}"
fail() {
  echo "FAIL: $*" >&2
  if [ "$SOFT" = "1" ]; then
    echo "(VERIFY_BY_TAG_SOFT=1 → reporting only; the out-of-repo prerequisite" >&2
    echo " above is the signal, not a flake. Exiting 0.)" >&2
    exit 0
  fi
  exit 1
}

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
cp -r . "$WORK/mod" || fail "could not copy module to scratch dir"
cd "$WORK/mod" || fail "could not enter scratch dir"

# Drop EVERY local gpufleet-* replace so resolution goes by tag.
# (rca carries no replace today — this loop is a future-proof guard.)
for rep in $(go mod edit -json | python3 -c \
  'import json,sys;[print(r["Old"]["Path"]) for r in (json.load(sys.stdin).get("Replace") or []) if r["Old"]["Path"].startswith("github.com/rocker-zhang/")]'); do
  echo ">> dropping replace: $rep"
  go mod edit -dropreplace "$rep"
done

echo ">> go mod download (by tag) ..."
go mod download "$GEN_MOD" || fail "cannot resolve ${GEN_MOD}@${GEN_VER} by tag (need proto tag gen/go/${GEN_VER} published)"

echo ">> go build ./... (by tag, no replace) ..."
go build ./... || fail "by-tag build failed after download"

# Build skips *_test.go; several packages import the gen module only from test
# files, so a test-only replace would slip past a build-only check. Run tests too.
echo ">> go test ./... (by tag, no replace) ..."
go test ./... || fail "by-tag test build/run failed"

echo ">> asserting go.sum now carries the gen module checksum ..."
grep -q "^${GEN_MOD} ${GEN_VER} " go.sum || fail "go.sum missing ${GEN_MOD} ${GEN_VER}"
grep -q "^${GEN_MOD} ${GEN_VER}/go.mod " go.sum || fail "go.sum missing ${GEN_MOD} ${GEN_VER}/go.mod"
echo "OK: by-tag release path resolves and go.sum carries ${GEN_MOD} ${GEN_VER}"
