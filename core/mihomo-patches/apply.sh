#!/bin/sh
set -eu
patch_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$patch_dir/../src/foss/golang/clash"
if git apply --reverse --check "$patch_dir/tailnet.patch" >/dev/null 2>&1; then
    exit 0
fi
git apply --check "$patch_dir/tailnet.patch"
git apply "$patch_dir/tailnet.patch"
