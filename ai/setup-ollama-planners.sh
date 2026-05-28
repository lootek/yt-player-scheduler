#!/usr/bin/env bash
# Pull alternative-to-gemma planning models and create agentic variants.
#
# After creation, base model tags can be removed safely: Ollama uses
# content-addressable blob storage, and the agentic tags reference the
# same blobs as the base tags. Removing a base tag drops only its
# manifest, not the underlying weights.
#
# Usage:
#   ./setup-ollama-planners.sh              # pull + create agents
#   ./setup-ollama-planners.sh --cleanup    # also remove base tags at the end
#   ./setup-ollama-planners.sh --lean       # only the top-3 picks
#   ./setup-ollama-planners.sh --print-only # skip pull/create, just print config
#
# Disk: ~110 GB for the full set, ~47 GB for --lean.

set -euo pipefail

CLEANUP=0
LEAN=0
PRINT_ONLY=0
for arg in "$@"; do
    case "$arg" in
        --cleanup)    CLEANUP=1 ;;
        --lean)       LEAN=1 ;;
        --print-only) PRINT_ONLY=1 ;;
        *) echo "unknown flag: $arg" >&2; exit 2 ;;
    esac
done

# base model -> agent tag, SYSTEM prompt is shared
if [[ $LEAN -eq 1 ]]; then
    MODELS=(
        "qwen2.5-coder:14b|qwen-coder-planner:14b"
        "qwen3:30b-a3b|qwen3-planner:30b-a3b"
        "qwen2.5-coder:32b|qwen-coder-planner:32b"
    )
else
    MODELS=(
        "qwen2.5-coder:14b|qwen-coder-planner:14b"
        "qwen2.5-coder:32b|qwen-coder-planner:32b"
        "qwen3:14b|qwen3-planner:14b"
        "qwen3:30b-a3b|qwen3-planner:30b-a3b"
        "deepseek-coder-v2:16b|deepseek-planner:16b"
        "mistral-small:24b|mistral-planner:24b"
        "codestral:22b|codestral-planner:22b"
        "phi4:14b|phi4-planner:14b"
    )
fi

SYSTEM_PROMPT='You are a senior engineer producing implementation plans. Cite specific file paths and line numbers from the codebase. Prefer additive changes over modifying existing signatures. Flag deployment/runtime gotchas explicitly. Be terse.'

NUM_CTX=16384
TEMPERATURE=0.2

if [[ $PRINT_ONLY -eq 0 ]]; then
    echo "==> Step 1: pull base models"
    for entry in "${MODELS[@]}"; do
        base="${entry%%|*}"
        if ollama list | awk '{print $1}' | grep -qx "$base"; then
            echo "    [skip] $base already present"
        else
            echo "    [pull] $base"
            ollama pull "$base"
        fi
    done

    echo
    echo "==> Step 2: create agentic variants (num_ctx=$NUM_CTX, temp=$TEMPERATURE)"
    for entry in "${MODELS[@]}"; do
        base="${entry%%|*}"
        agent="${entry##*|}"
        echo "    [create] $agent  <-  $base"
        ollama create "$agent" -f - <<EOF
FROM $base
PARAMETER num_ctx $NUM_CTX
PARAMETER temperature $TEMPERATURE
SYSTEM "$SYSTEM_PROMPT"
EOF
    done

    if [[ $CLEANUP -eq 1 ]]; then
        echo
        echo "==> Step 3: remove base tags (blobs stay; agent tags keep working)"
        for entry in "${MODELS[@]}"; do
            base="${entry%%|*}"
            echo "    [rm] $base"
            ollama rm "$base" || true
        done
    else
        echo
        echo "==> Skipping cleanup. Re-run with --cleanup to drop base tags."
    fi

    echo
    echo "==> Available planners:"
    ollama list | awk 'NR==1 || /-planner:/'
fi

echo
echo "==> Claude Code settings.json snippet"
echo "    Paste into ~/.claude/settings.json (or project .claude/settings.json)."
echo "    Switch active model with: /model <name>   (e.g. /model qwen-coder-planner:14b)"
echo "    Assumes ollama serving on default 127.0.0.1:11434 with OpenAI-compat shim."
echo
echo "----- BEGIN settings.json snippet -----"
{
    printf '{\n'
    printf '  "env": {\n'
    printf '    "ANTHROPIC_BASE_URL": "http://127.0.0.1:11434/v1",\n'
    printf '    "ANTHROPIC_AUTH_TOKEN": "ollama",\n'
    printf '    "ANTHROPIC_MODEL": "%s",\n' "${MODELS[0]##*|}"
    printf '    "ANTHROPIC_SMALL_FAST_MODEL": "%s"\n' "${MODELS[0]##*|}"
    printf '  },\n'
    printf '  "models": {\n'
    last_idx=$(( ${#MODELS[@]} - 1 ))
    for i in "${!MODELS[@]}"; do
        agent="${MODELS[$i]##*|}"
        sep=","
        [[ $i -eq $last_idx ]] && sep=""
        printf '    "%s": {\n' "$agent"
        printf '      "provider": "ollama",\n'
        printf '      "model": "%s",\n' "$agent"
        printf '      "baseURL": "http://127.0.0.1:11434/v1"\n'
        printf '    }%s\n' "$sep"
    done
    printf '  }\n'
    printf '}\n'
}
echo "----- END settings.json snippet -----"
echo
echo "Note: Claude Code's native Anthropic-API client does not speak Ollama"
echo "directly. To use these models, you need ONE of:"
echo "  - claude-code-router  (https://github.com/musistudio/claude-code-router)"
echo "  - LiteLLM proxy with Anthropic compatibility"
echo "  - Ollama's built-in OpenAI-compat endpoint + a shim (e.g. y-cli, anyclaude)"
echo "The 'env' block above targets the most common pattern (OpenAI-compat shim);"
echo "the 'models' block is consumed by claude-code-router-style configs."
