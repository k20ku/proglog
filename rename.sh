#!/usr/bin/bash

MOD=github.com/k20ku/proglog

# GNU sed (Linux). On macOS use: sed -i '' -E ...
for pkg in log auth discovery server agent config; do
  grep -rl --include='*.go' "$MOD/internal/$pkg" . \
    | xargs sed -i -E "s#\"$MOD/internal/$pkg(/[^\"]*)?\"#\"$MOD/$pkg\1\"#g"
done
