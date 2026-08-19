# AI planning bake-off: 23 models, 25 runs, one task

I gave the same brownfield feature request to as many LLMs as I could and compared the
implementation plans, not the generated code.

## The task

Extend this service (pinned at `bfe645af`, deliberately not HEAD) with a web UI: paste a
YouTube video / channel / playlist URL, it downloads to a configured directory, two checkboxes
(schedule via MPD, video vs music-only), consistent with the legacy `~/scripts/yt.sh` naming
and flags. Exact prompt: [`prompt-plan.md`](prompt-plan.md).

Every model ran in plan mode inside an agentic harness with shell access to the repo and, where
the harness allowed, SSH to the Raspberry Pi this actually runs on. So the plans differ not only in
design taste but in how much each model actually went and checked.

## Results

[View the comparison table](https://lootek.github.io/yt-player-scheduler/ai-plans/model-comparison.html)
— 25 runs x 13 categories, toggle models, hover any cell for the original finding.
GitHub serves raw HTML in-repo as plain text, so that link goes via Pages. Or clone and open
`model-comparison.html` locally.

Regenerate from the data: `python3 make_comparison_table.py`

- [`plans/`](plans/) - the 25 plans, verbatim
- [`plan-comparison.csv`](plan-comparison.csv) - the scored matrix

## What separated them

Not parameter count, and not tokens burned:

1. Verifying vs assuming - the biggest divider by far. This repo's `master` already has a
   different implementation of this feature, live on the Pi. Three models noticed. One curl'd
   the running service and found its job history was write-only (117 events on disk,
   `/api/jobs` returning `[]`), then flagged three unrelated pre-existing bugs nobody asked about.
2. MPD semantics - "schedule for playing" means append to the queue. Several plans reached for
   the existing `PlayWithMPD`, which interrupts whatever's playing.
3. Legacy naming fidelity - reproducing `yt.sh`'s nested output template exactly, including the
   odd `NA` directory for single videos, which is pre-existing and shouldn't be "fixed".
4. Additive vs invasive - adding a sibling method versus refactoring the function the cron
   scheduler depends on.
5. Deployment reality - the container runs `network_mode: host`, so adding a `ports:` mapping is
   wrong, and the bind mount needs to widen for downloads to land on the host tree.

⇄ / ⛒ `fable-5 v1` and `v2` are the same model on the same prompt, kept on purpose. v1 hit a
tool outage that blocked `git show` and SSH, so it reconstructed the target commit from the reflog.
v2 re-ran clean and verified everything, with 42% fewer requests and 37% fewer tokens, while
generating 25% more output. A model score is partly a score of its environment.

## Caveats - read before quoting a number

This is one engineer's opinionated comparison, not a benchmark. n=1 per model, and the scorer
isn't blind - I wrote the task. Runs span weeks, different machines, and a slightly refined prompt
between batches. `muse-glimmer` is absent: it never parsed the prompt in this harness, so there was
nothing to score.

Two things worth knowing about the numbers. The 13 categories are not 13 independent dimensions -
they correlate strongly (first principal component ~53% of variance, alpha 0.92), so read the table
as one axis, roughly "did the model go and check the running system", measured several ways. And the
ratings are keyword-mapped from prose cells, which is inspectable but brittle: I've found and fixed
18 scoring bugs so far, every one of them a negation or an incidental substring - at one point three
plans that explicitly broke the deployment were showing full marks for deployment. Assume more
remain.

The top two tie on the rubric (51/52 each), so #1 vs #2 is my judgement, not a measurement.

For a rigorous take on this kind of evaluation (repeat trials, median + pass@k, a fixed judge
stronger than every entrant, pinned containers), see
[`przeprogramowani/10x-bench`](https://github.com/przeprogramowani/10x-bench).

The plans are published verbatim, including `192.168.10.22` and local paths. That address is
RFC1918 private, and editing model output would defeat the point of publishing it as evidence.

MIT, same as the rest of the repo.
