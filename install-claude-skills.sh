#!/usr/bin/env bash

set -euo pipefail

TARGET=".claude/skills"
TMP="$(mktemp -d)"

trap 'rm -rf "$TMP"' EXIT

echo "Installing Claude Code skills into: $TARGET"

mkdir -p "$TARGET"

install_skill() {
    local source="$1"
    local name="$2"

    echo "Installing: $name"

    rm -rf "$TARGET/$name"
    cp -R "$source" "$TARGET/$name"
}

# ============================================================
# 1. Engineering Workflow
#
# Includes:
# - state-transition-testing
# - combinatorial-testing
# - decision-tables
# - scenario-analysis
# - scenario-design
# - blind-spot-coverage
# - cause-effect-graphing
# - equivalence-partitioning-bva
# - etc.
# ============================================================

echo
echo "Cloning sliekens/agentic..."

git clone \
    --depth 1 \
    https://github.com/sliekens/agentic.git \
    "$TMP/agentic"

ENGINEERING_SKILLS="$TMP/agentic/plugins/engineering-workflow/skills"

for skill_dir in "$ENGINEERING_SKILLS"/*; do
    [ -d "$skill_dir" ] || continue

    skill_name="$(basename "$skill_dir")"
    install_skill "$skill_dir" "$skill_name"
done


# ============================================================
# 2. nWave property-based-testing
# ============================================================

echo
echo "Cloning nWave-ai/nWave..."

git clone \
    --depth 1 \
    https://github.com/nWave-ai/nWave.git \
    "$TMP/nwave"

install_skill \
    "$TMP/nwave/nWave/skills/nw-property-based-testing" \
    "nw-property-based-testing"

# Upstream nWave uses this skill internally and currently has:
#
#   user-invocable: false
#   disable-model-invocation: true
#
# For standalone usage in our project we want Claude to be
# able to discover and invoke it automatically.
NW_SKILL="$TARGET/nw-property-based-testing/SKILL.md"

sed -i.bak \
    -e '/^user-invocable:[[:space:]]*false[[:space:]]*$/d' \
    -e '/^disable-model-invocation:[[:space:]]*true[[:space:]]*$/d' \
    "$NW_SKILL"

rm -f "$NW_SKILL.bak"


# ============================================================
# 3. Java skills
#
# Includes:
# - java-testing
# - java-development
# - java-concurrency
# - java-maven
# - java-observability
# - etc.
# ============================================================

echo
echo "Cloning mtkhawaja/java-skills..."

git clone \
    --depth 1 \
    https://github.com/mtkhawaja/java-skills.git \
    "$TMP/java-skills"

for skill_dir in "$TMP/java-skills/skills"/*; do
    [ -d "$skill_dir" ] || continue

    skill_name="$(basename "$skill_dir")"
    install_skill "$skill_dir" "$skill_name"
done


# ============================================================
# 4. Mutation testing / PIT
# ============================================================

echo
echo "Cloning jvm-skills/jvm-skills..."

git clone \
    --depth 1 \
    https://github.com/jvm-skills/jvm-skills.git \
    "$TMP/jvm-skills"

install_skill \
    "$TMP/jvm-skills/.claude/skills/mutation-testing" \
    "mutation-testing"


# ============================================================
# Verification
# ============================================================

echo
echo "=========================================="
echo "Installed Claude skills:"
echo "=========================================="

find "$TARGET" \
    -mindepth 2 \
    -maxdepth 2 \
    -name SKILL.md \
    -print \
    | sed "s#$TARGET/##" \
    | sed 's#/SKILL.md##' \
    | sort

echo
echo "Important skills:"
echo

for skill in \
    state-transition-testing \
    combinatorial-testing \
    decision-tables \
    scenario-analysis \
    blind-spot-coverage \
    nw-property-based-testing \
    java-testing \
    mutation-testing
do
    if [ -f "$TARGET/$skill/SKILL.md" ]; then
        echo "  ✓ $skill"
    else
        echo "  ✗ $skill"
    fi
done

echo
echo "Done."
echo
echo "Restart Claude Code if .claude/skills did not exist"
echo "when the current Claude session was started."