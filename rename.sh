#!/usr/bin/bash

OLD_MOD=github.com/k20ku/proglog
NEW_MOD=github.com/k20ku/epilog

go mod edit -module "$NEW_MOD"

grep -rl --include='*.go' "$OLD_MOD" . | xargs sed -i "s#$OLD_MOD#$NEW_MOD#g"
