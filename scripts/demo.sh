#!/usr/bin/env bash
# Regenerates the committed example report (examples/coverage.example.json)
# from the sample fixture.
#
# The fixture's coverage is generated from a scratch copy under a neutral path
# so the report's `root` field does not record the machine that produced it.
# Node and npm are needed for the fixture's vitest run; `npm install` is run
# inside the copy on first use.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture_src="${repo_root}/examples/sample-go-ts"
work="${TMPDIR:-/tmp}/cover100-example"
fixture="${work}/sample-go-ts"

rm -rf "${work}"
mkdir -p "${work}"
cp -R "${fixture_src}" "${fixture}"
rm -rf "${fixture}/web/node_modules"

echo "==> installing the fixture's test dependencies"
(cd "${fixture}/web" && npm install --no-audit --no-fund >/dev/null)

echo "==> building cover100"
go build -o "${work}/cover100" "${repo_root}/cmd/cover100"

echo "==> collecting coverage"
"${work}/cover100" "${fixture}" --no-serve --no-open --out "${work}/out/coverage.json" >/dev/null

cp "${work}/out/coverage.json" "${repo_root}/examples/coverage.example.json"
echo "==> wrote examples/coverage.example.json"
