# AI planning bake-off — 23 models, 25 runs, one identical task

An experiment: give **the same brownfield feature request** to as many LLMs as possible and
compare the *implementation plans* they produce — not generated code.

## The task

Extend this service (pinned at commit **`bfe645af`**, deliberately *not* HEAD) with a web UI
where you paste a YouTube video / channel / playlist URL and it downloads to a configured
directory, with two checkboxes: *schedule for playing through MPD* and *video vs music-only* —
staying consistent with the legacy `~/scripts/yt.sh` naming and yt-dlp flags.

Exact prompts: [`prompt-plan.md`](prompt-plan.md) (planning) and
[`prompt-implementation.md`](prompt-implementation.md).

Every model ran in **plan mode** inside an agentic harness (Claude Code, or the equivalent for
locally-served models) with shell access to the repo and — where the harness allowed — SSH to the
Raspberry Pi the service actually runs on. So the plans differ not just in design taste but in
**how much each model went and checked**.

## Results

Open **[`model-comparison.html`](model-comparison.html)** in a browser — a self-contained,
dependency-free table (dark). Toggle models, hover any cell for the original finding.

Regenerate it from the data:

```sh
python3 make_comparison_table.py     # reads plan-comparison.csv -> model-comparison.html
```

- [`plans/`](plans/) — the 25 plans, verbatim as the models wrote them
- [`plan-comparison.csv`](plan-comparison.csv) — the scored matrix (13 categories + token counts)

## What actually separated the plans

Not parameter count, and not tokens burned. The dividers were:

1. **Did it verify, or assume?** The single biggest differentiator. The repo's `master` already
   contains a *different* implementation of this same feature, live on the Pi. Three models found
   that; **one went further and curl'd the running service**, discovering its job history was
   write-only (117 events on disk, `/api/jobs` returning `[]`). It also flagged three unrelated
   pre-existing bugs nobody asked about.
2. **MPD semantics** — the request says "schedule for playing". Correct reading is *append to the
   queue*; several plans reached for the existing `PlayWithMPD`, which interrupts whatever is
   playing. A few went further and resume the prior track at its saved position.
3. **Legacy naming fidelity** — reproducing `yt.sh`'s nested output template exactly. Some models
   substituted their own scheme, or "fixed" the `NA` directory that appears for single videos
   (which is pre-existing behaviour, so changing it breaks consistency).
4. **Additive vs invasive** — adding a sibling method versus refactoring the function the cron
   scheduler depends on.
5. **Deployment reality** — the container uses `network_mode: host`, so adding a `ports:` mapping
   is wrong, and the bind mount has to be widened for downloads to land on the host tree.

## Reading the markers

- **⇄ / ⛒ `fable-5 v1` and `fable-5 v2`** are the *same model on the same prompt*, kept on
  purpose. v1 (⛒) hit a tool outage that blocked `git show` and SSH, so it reconstructed the
  target commit from the reflog. v2 re-ran cleanly and verified everything — using **42% fewer
  requests and 37% fewer tokens while generating 25% more output**. A useful reminder that a
  "model score" is partly a score of the environment it ran in.
- **⚠** — model has since been retired by its host, so the result stands as history but can't be
  re-run.

## Caveats — read before quoting any number

This is **one engineer's opinionated comparison, not a benchmark.** Specifically:

- **n=1 per model.** No repeat runs, so no variance and no confidence intervals.
- **The scorer is not blind** and is the same person who wrote the task.
- The 0–100 "plan quality" is **derived from the assigned tier**, so it is not independent of the
  ranking it appears to justify. Category ratings (0–4) are the more defensible signal.
- Ratings are mapped from prose findings by keyword rules — inspectable, but brittle.
- Runs happened over several weeks across different machines and harness versions, and the prompt
  was refined slightly between early and later batches.
- One model (`muse-glimmer`) is absent entirely: it never parsed the prompt in this harness and
  produced no plan, so there was nothing to score.

For a genuinely rigorous take on this kind of evaluation — repeat trials, median + pass@k, a fixed
judge stronger than every entrant, pinned containers — see
[`przeprogramowani/10x-bench`](https://github.com/przeprogramowani/10x-bench).

## Note on the contents

The plans are published **verbatim**, including references to `192.168.10.22`, `pi@ithilien` and
local filesystem paths. That address is RFC1918 private (non-routable), and editing model output
would defeat the point of publishing it as evidence.

MIT, same as the rest of the repo.
