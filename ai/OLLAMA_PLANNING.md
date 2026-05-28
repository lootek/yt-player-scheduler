# Ollama-driven Claude Code planning — model selection & context notes

Hardware: MacBook Pro M1, 32 GB unified memory.

---

## Model recommendations (alternatives to gemma4)

Real ceiling on M1 32G: ~22 GB of weights (Q4_K_M) before paging hurts. Memory bandwidth on plain M1 (~68 GB/s) also matters — bigger ≠ better if it crawls.

| Tier | Model | Size (Q4_K_M) | Why |
|---|---|---|---|
| **Top pick** | `qwen2.5-coder:32b` | ~20 GB | Strongest open coding model in this weight class; planning + tool-call quality close to Sonnet on code-shaped tasks. Slow on M1 (~5-8 t/s) but the output is worth it. |
| **Best speed/quality** | `qwen2.5-coder:14b` | ~9 GB | Punches above its size, runs fast (~20 t/s), leaves room for context. Default daily driver. |
| **Reasoning-heavy** | `qwen3:14b` or `qwen3:32b` | ~9 / 20 GB | Newer, stronger at multi-step reasoning than 2.5 line. Use for planning, not raw codegen. |
| **MoE wildcard (new)** | `qwen3:30b-a3b` | ~18 GB | 30B total / 3B active — 30B-class quality at 3B-class speed. Excellent fit for M1. |
| **MoE wildcard (older)** | `deepseek-coder-v2:16b` (Lite) | ~9 GB | 16B total / 2.4B active — feels like a 30B but runs like a 7B. |
| **Dense generalist** | `mistral-small:22b` (or `:24b`) | ~13 GB | Solid all-rounder, good instruction-following, decent tool use. |
| **Code specialist** | `codestral:22b` | ~13 GB | Mistral's code model, strong on edits/refactor planning. License is non-commercial — fine for personal use. |
| **Tiny but smart** | `phi4:14b` | ~9 GB | Microsoft's 14B, strong reasoning per param. Worse at code than Qwen-Coder, better at structured planning. |

### Avoid
- `llama3.3:70b` — won't fit at usable quant.
- `gemma2:27b` / `gemma3:27b` — same family you're already finding weak; no reason to expect step-change.
- Q8 quants of anything ≥14B — quality gain is marginal, speed/RAM hit isn't.

---

## Did the gemma `num_ctx 32768` Modelfile actually help?

Empirically, the two outputs are a wash:

| Aspect | gemma4-26b | gemma4-agent-26b | Better? |
|---|---|---|---|
| Project name | `yt-daily-player` ✓ | `yt-player-scheduler` (invented) | base wins |
| yt-dlp output template | absent | `%(uploader)s/%(playlist_title)s/...` ✓ | agent wins |
| `video_only` flag explicit | no | yes ✓ | agent wins |
| File count | 4 | 5 | tie |
| Verification | 2 steps | 4 steps | agent slightly better |
| Architecture depth | same | same | tie |
| Sync vs async | sync | sync | tie (both wrong) |

Net: a wash. One picks up a detail, the other gets the project name right. No structural improvement.

### Why the bump *should* have mattered (and probably did, just invisibly)

- **Ollama's default `num_ctx` is 2048**, not 8192. Claude Code's system prompt + tool schemas + skill definitions easily run 15-25K tokens. At 2048, Ollama silently sliding-windows the input — meaning base `gemma4:26b` was almost certainly seeing only the tail end of the prompt while everything before it got truncated.
- **Truncation is tail-preserving.** The user's instruction survives in both cases. What's lost is the *grounding* — repo exploration, file content, prior turns. So both runs got the same instruction; only the `agent` variant saw the full context.
- **Why no visible quality lift then?** The model is the bottleneck, not the context. Gemma 26B at this task is just not strong enough — feeding it more material to reason over doesn't unlock better reasoning. The agent variant did pick up the `yt.sh` naming pattern (likely because the longer window saw the legacy script reference), but lost the project name and missed everything else.
- **claude-code → Ollama adapter caveat.** Depending on the adapter (claude-code-router, OpenAI-compat proxy, etc.), it may or may not respect the Modelfile's `num_ctx`. Verify with `ollama ps` mid-request — it'll show the actual loaded context size.

### Practical implication

- Setting `num_ctx 32768` is the right move; do it for every model you use with claude-code.
- Better still, set it via env: `OLLAMA_CONTEXT_LENGTH=32768 ollama serve` so you don't need per-model Modelfiles.
- But context fixes truncation, not capability. The next gain is from switching off gemma — qwen2.5-coder family will respond to the larger context far more visibly because it can actually use the grounding.

### What would actually improve planning quality

1. Switch model family (qwen2.5-coder beats gemma for code-shaped planning).
2. Add a `system` message with explicit planning conventions / file-citation requirement / async-vs-sync guidance.
3. Lower `temperature` (e.g. 0.2) for planning — gemma defaults are chatty.
4. Set `num_predict` high enough that the plan doesn't get cut off.

### Suggested replacement Modelfile

A drop-in replacement for the `gemma-agent` Modelfile, but built on a stronger base model with a few extra tunings baked in. Same workflow:

```sh
ollama create qwen-planner:14b -f - <<'EOF'
FROM qwen2.5-coder:14b
PARAMETER num_ctx 16384
PARAMETER temperature 0.2
SYSTEM "You are a senior engineer producing implementation plans. Cite specific file paths and line numbers from the codebase. Prefer additive changes over modifying existing signatures. Flag deployment/runtime gotchas explicitly."
EOF
```

Then point claude-code at `qwen-planner:14b` instead of `gemma-agent:26b`.

What each line does:

| Directive | Effect |
|---|---|
| `FROM qwen2.5-coder:14b` | Base weights — the actual model that does the thinking. Swap for `:32b`, `qwen3:30b-a3b`, etc. depending on which one wins your bake-off. |
| `PARAMETER num_ctx 16384` | Same lever as the gemma-agent — overrides Ollama's 2048 default so claude-code's full prompt isn't truncated. 16K is enough headroom without burning RAM needed for weights. |
| `PARAMETER temperature 0.2` | Lower = more deterministic, less chatty. Planning benefits from this; brainstorming doesn't. Default for most models is 0.7-0.8. |
| `SYSTEM "..."` | Persistent system prompt baked into every conversation with this model. Sonnet/Opus's planning quality partly comes from Claude Code injecting heavy planning-shaped system prompts; this is the poor-man's version for local models. |

Why this is more than the gemma-agent's `num_ctx` bump:

- The gemma-agent Modelfile only fixed truncation. This one *also* nudges the model toward planning behavior (lower temp, system prompt with explicit "cite files, flag gotchas, prefer additive" rules).
- The system prompt is the biggest lever. Iterate on just that string — re-run `ollama create` with a tweaked SYSTEM line and you've got a new "agent" without re-pulling weights.

**Tip: keep multiple variants** — `qwen-planner:14b`, `qwen-codegen:14b` (different SYSTEM, higher temp), `qwen-explainer:14b` — and switch via claude-code config. Same base weights, different personas, ~zero extra disk.

That system prompt alone will likely do more than the 8K→32K bump did.

---

## When `num_ctx` tweaks actually move the needle

1. Multi-turn agent loops where tool results stack up (Claude Code style — yes, this is you).
2. RAG/long-doc summarization where source clearly exceeds 8K.
3. Long codebases pasted inline.
