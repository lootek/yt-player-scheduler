#!/usr/bin/env python3
"""Generate a self-contained dark-mode HTML comparison table from plan-comparison.csv.

Rows = scoring categories, columns = models (toggleable). Ratings are 0-4 meters
derived from the CSV text by explicit keyword rules (inspectable, reproducible),
with the original cell text kept as the hover tooltip so nothing is lost.
Palette: dataviz reference instance, dark surface #1a1a19, ordinal blue ramp
(validated: monotone L, adjacent dL>=0.06, light-end 2.15:1, single hue).
"""
import csv, html, json, os, re

HERE = os.path.dirname(os.path.abspath(__file__))
SRC = os.path.join(HERE, "plan-comparison.csv")
OUT = os.path.join(HERE, "model-comparison.html")

# Plan quality 0-100 is computed BOTTOM-UP from the scored categories, weighted by how
# much each mattered to this brief. It is NOT derived from the rank (the previous version
# was, which made it circular: it could not independently justify the ordering it fed).
# Weights: the brief's explicit asks (grounding, MPD semantics, yt.sh fidelity) carry most;
# auth was never requested, so it barely counts.
WEIGHTS = {
    "Verified vs assumed": 3.0, "Queue vs interrupt": 3.0, "yt.sh fidelity": 3.0,
    "Cron path left intact": 2.0, "Async / concurrency": 2.0, "Persistence": 2.0,
    "Automated tests": 2.0, "Avoided ports: mapping": 1.5, "Crash resilience": 1.5,
    "Checkbox model": 1.0, "Video container": 1.0, "Audio format": 0.5, "Auth": 0.5,
    # new axes: discovery + execution matter to this brief; boot-guard is a real shipped bug;
    # safety is the most independent axis in the matrix
    "Discovery facts stated": 2.0, "Execution plan": 2.0,
    "Boot-guard awareness": 1.5, "Input/command safety": 1.5,
}

# Models retired from Ollama Cloud (waves 2026-07-15 / 07-31) - can't be re-run/demoed.
RETIRED = {"gemini-3-flash-preview-cloud", "qwen3-coder-480b-cloud"}
# Runs handicapped by environment failure, not model capability (shown with a marker).
HANDICAPPED = {"fable-5 v1"}   # original: Bash-tool outage blocked git show/ssh -> reflog-reconstructed grounding
# Same model + same prompt, run twice under different tool availability. Kept as a PAIR
# on purpose: it shows how much of a "model result" is really an environment result.
PAIRED = {"fable-5 v1", "fable-5 v2"}

# Four axes added 2026-08-19 to cover strengths the original 13 missed (notably opus-4-8's
# verified-facts block and its commit/subagent plan). These are HAND-SCORED from a full read of
# all 25 plans - not keyword-derived - so they carry an evidence phrase instead of a prose cell.
NEW_CATS = [
    ("Discovery facts stated", "discovery_facts", "discovery_evidence"),
    ("Execution plan",         "execution_plan",  "execution_evidence"),
    ("Boot-guard awareness",   "boot_guard",      "boot_evidence"),
    ("Input/command safety",   "input_safety",    "safety_evidence"),
]
NEW_SCORES = {}
_np = os.path.join(HERE, "new-categories-scores.csv")
if os.path.exists(_np):
    with open(_np) as _f:
        for _r in csv.DictReader(_f):
            NEW_SCORES[_r["plan"]] = _r

L = lambda s: s.lower()
NEG = r"(no|not|never|without)\s+"   # negation prefix used by the corrected scorers

def sc_grounded(t):
    t=L(t)
    if re.search(NEG+r"(ssh|verif|investigat)",t) or "assumed" in t: return 0,"none/shallow"
    if "shallow" in t: return 0,"none/shallow"
    if "restated" in t: return 1,"restated only"
    if "deepest" in t and "verified" in t: return 4,"verified, deepest"
    if "reflog" in t or "reconstruct" in t: return 2,"reconstructed"
    if "deepest" in t: return 4,"deepest"
    if "file:line" in t or "ssh" in t or "verified" in t: return 4,"verified"
    if re.search(r"no commit hash",t): return 2,"deep, unpinned"
    if "commit" in t or "cites" in t or "precise" in t: return 3,"commit-precise"
    if "deep" in t: return 3,"deep"
    if "none" in t: return 0,"none/shallow"
    return 2,"partial"
def sc_mpd(t):
    t=L(t)
    if not t.strip() or t.strip() in ("none","vague","unclear"): return 0,"not addressed"
    if "cron-play" in t or ("cron" in t and "misread" in t): return 0,"cron-play misread"
    if "playwithmpd" in t and "enqueue" not in t and "append" not in t and "untouched" not in t: return 1,"interrupts"
    if "resume prior" in t or "resume the previously" in t: return 4,"append+resume (best)"
    if "updating_db" in t or ("enqueue" in t and "no play" in t): return 4,"append (best)"
    if "enqueue" in t or "append" in t or "addid" in t: return 3,"append"
    if "scheduler queue" in t or "muddl" in t: return 1,"misread"
    return 2,"unclear"
def sc_checkboxes(t):
    t=L(t)
    n=re.search(r"(\d)",t)
    if "coupled" in t or "conflat" in t or "tied" in t: return 1,"coupled"
    if "no video" in t or "missing" in t: return 0,"missing one"
    if not n: return 0,"not modelled"
    v=int(n.group(1))
    if v>=2: return 4,f"{v} independent"
    return 0,"missing one"
def sc_ytsh(t):
    t=L(t)
    if "not addressed" in t or "not fully" in t or not t.strip(): return 0,"not addressed"
    if "full" in t and "existing" in t and "archive" in t: return 4,"full + shared ledger"
    if "full" in t and ("deliberately keeps" in t or "don't fix" in t): return 4,"full, NA kept"
    if "full" in t and ("fix" in t or "|)" in t): return 4,"full + NA fix"
    if "full" in t: return 4,"full fidelity"
    if "wrong" in t or "overrid" in t or "existing scheduler" in t: return 0,"wrong template"
    if "no archive" in t or "no playlist_title" in t or "drops" in t or "partial" in t: return 1,"partial"
    if "template only" in t or "template" in t: return 2,"template only"
    return 2,"partial"
def sc_newmethod(t):
    t=L(t)
    if re.search(r"(refactor|delegat|modifies|modify)\w*\s+(the\s+)?(existing\s+)?download|modifies download|refactors existing download|modifies pattern|adds videoonly",t): return 1,"modifies Download"
    if re.search(r"download\s+untouched|existing download.*untouched|untouched.*download",t): return 4,"additive"
    if "wrapper" in t or "sibling" in t: return 4,"additive"
    if "untouched" in t: return 4,"additive"
    if "refactor" in t or "delegat" in t or "modifies" in t or "modify" in t: return 1,"modifies Download"
    if "reuses existing download" in t or "python" in t or "flask" in t: return 0,"no new method"
    if re.search(r"^download\w+|new method|\bnew\b",t): return 2,"new name, silent on Download"
    return 2,"unclear"
def sc_async(t):
    t=L(t)
    if re.search(r"\b(sync|synchronous)\b",t) or t.strip()=="none" or "no queue" in t: return 0,"synchronous"
    pool="worker pool" in t or "semaphore" in t or re.search(r"\bworkers?\b",t)
    if pool and "timeout" in t: return 4,"pool + timeout"
    if pool: return 3,"worker pool"
    if "queue" in t: return 3,"queue"
    if "goroutine" in t or "background" in t: return 2,"background"
    return 2,"partial"
def sc_auth(t):
    t=L(t)
    if t.strip() in ("none","no auth","-","mentioned") or "no auth" in t: return 0,"none"
    if "defer" in t or "omit" in t or re.search(r"\bif (the )?(ui )?(is )?(exposed|a concern)",t): return 0,"deferred"
    if re.search(r"^optional$",t.strip()): return 1,"named only"
    mandatory=("fatal" in t or "refuse" in t) and not re.search(r"no\s+refuse|not\s+mandator",t)
    if mandatory: return 4,"mandatory"
    if "constant-time" in t or "subtle" in t: return 4,"optional + hardened"
    if "optional" in t or "basic auth" in t or "auth" in t: return 3,"optional"
    return 1,"weak"
def sc_hostnet(t):
    t=L(t)
    if re.search(r"break|adds ports|8080:8080|:5000|expose 8080",t) and "no ports" not in t:
        if "redundant" in t or "expose" in t and "ports" not in t: return 2,"redundant expose"
        return 0,"breaks host-net"
    if "not addressed" in t or "no docker" in t or "gap" in t: return 1,"not addressed"
    if "no ports" in t or "no port mapping" in t: return 4,"correct"
    if "expose" in t: return 2,"redundant expose"
    if re.search(r"\bports\b",t): return 0,"breaks host-net"
    return 2,"unclear"
def sc_persist(t):
    t=L(t)
    if re.search(NEG+r"(file|durable|persist|db)",t) or "no on-disk" in t: return 0,"none"
    if "opt-in" in t and "not default" in t: return 2,"opt-in durable"
    if "reconcil" in t or ("jsonl" in t and "crash" in t): return 4,"durable + reconcile"
    if "jsonl" in t or "durable" in t or re.search(r"\bfile\b|history_path",t): return 3,"durable"
    if "bounded" in t or "in-mem" in t or "map" in t or "cap" in t: return 1,"in-memory"
    if "none" in t: return 0,"none"
    return 1,"in-memory"
def sc_tests(t):
    t=L(t)
    if re.search(r"no (automated|unit)",t) or "manual only" in t: return 1,"manual only"
    if "optional" in t and ("httptest" in t or "unit" in t or "test" in t): return 2,"tests optional"
    if "richest" in t or ("unit" in t and "e2e" in t): return 4,"unit + e2e"
    if "httptest" in t and "regression" in t: return 4,"unit + regression"
    if "httptest" in t or "unit" in t: return 3,"unit tests"
    if "regression" in t: return 3,"regression"
    if "manual" in t or "smoke" in t or "step" in t or "curl" in t: return 1,"manual only"
    return 0,"none stated"
def sc_crash(t):
    t=L(t)
    rec = re.search(r"recover|panic",t) and not re.search(r"no recover",t)
    mid = re.search(r"(panic-?recover|panic recovery)",t)
    if mid and "no recover() around workers" in t: return 3,"handler recover only"
    if rec: return 4,"recover()"
    if "reconcil" in t: return 4,"state reconcile"
    if "timeout" in t: return 2,"timeout only"
    if "graceful" in t or "shutdown" in t: return 1,"shutdown only"
    if t.strip() in ("none","not addressed"): return 0,"none"
    return 1,"minimal"
def sc_video(t):
    t=L(t)
    if re.search(r"no (video|merge)|mp4|--format best",t) and "mkv" not in t: pass
    if "mkv" in t and not re.search(r"no merge|no merge flag|not mkv",t): return 4,"mkv (correct)"
    if "mp4" in t: return 1,"mp4 (wrong)"
    if "no merge" in t or "format best" in t: return 0,"no merge"
    if "no video" in t: return 0,"no video option"
    if "unspecified" in t or "n/a" in t or not t.strip(): return 1,"unspecified"
    return 2,"partial"
def sc_audio(t):
    t=L(t)
    if re.search(NEG+r"m4a|avoids m4a",t): return 1,"wrong/absent"
    if "bestaudio" in t and ("native" in t or "no re-encode" in t): return 4,"native, no re-encode"
    if "m4a" in t: return 4,"m4a (correct)"
    if not t.strip() or "n/a" in t or "unspecified" in t: return 0,"omitted"
    return 2,"other"
CATS = [
    ("Verified vs assumed",    "Grounded",                         sc_grounded),
    ("Queue vs interrupt",     "MPD semantics",                    sc_mpd),
    ("Checkbox model",         "Checkboxes",                       sc_checkboxes),
    ("yt.sh fidelity",         "yt.sh template+archive",           sc_ytsh),
    ("Cron path left intact",  "New method vs modify Download",    sc_newmethod),
    ("Async / concurrency",    "Async queue",                      sc_async),
    ("Auth",                   "Auth",                             sc_auth),
    ("Avoided ports: mapping", "Host-net handling",                sc_hostnet),
    ("Persistence",            "Persistence",                      sc_persist),
    ("Automated tests",        "Tests",                            sc_tests),
    ("Crash resilience",       "Crash resilience",                 sc_crash),
    ("Video container",        "Video codec/format",               sc_video),
    ("Audio format",           "Audio codec/format",               sc_audio),
]

# Five CSV rows contain unquoted commas inside prose cells, so DictReader shifts the
# trailing numeric columns. The last 6 fields are always
# total,total%min,total%max,output,output%min,output%max -> index from the RIGHT,
# and map the descriptive columns positionally from the LEFT (they precede the drift).
NUM_TAIL = 6
HDR_IDX = {}

models = []
with open(SRC) as f:
    rows = list(csv.reader(f))
hdr = rows[0]
for i, h in enumerate(hdr):
    HDR_IDX[h] = i

assert all(len(r) == len(hdr) for r in rows[1:] if r), "CSV is ragged - re-run the comma repair"

for r in rows[1:]:
    if not r or not r[1].strip():
        continue
    plan = r[1]
    total_tok = int(r[HDR_IDX["total_tokens"]])
    out_tok = int(r[HDR_IDX["output_tokens"]])
    q = 0  # filled in after all cells are scored (needs WEIGHTS x cells)
    cells = {}
    for label, col, fn in CATS:
        raw = r[HDR_IDX[col]]
        score, short = fn(raw)
        cells[label] = {"s": score, "t": short, "raw": raw}
    for _label, _sc, _ev in NEW_CATS:
        _row = NEW_SCORES.get(plan)
        if _row:
            cells[_label] = {"s": int(_row[_sc]), "t": _row[_ev][:34],
                             "raw": _row[_ev] + "  [hand-scored from a full plan read]"}
        else:
            cells[_label] = {"s": 0, "t": "not scored", "raw": "no data"}
    models.append({
        "plan": plan, "rank": int(r[0]), "tier": r[2],
        "quality": q, "out": out_tok, "total": total_tok,
        "cells": cells, "retired": plan in RETIRED, "handicapped": plan in HANDICAPPED,
        "paired": plan in PAIRED,
    })

# --- bottom-up quality: weighted category sum, rescaled so the field's best = 99 ---
for m in models:
    num = sum(WEIGHTS[c] * m["cells"][c]["s"] for c in WEIGHTS)
    den = sum(WEIGHTS.values()) * 4
    m["quality"] = round(99 * num / den, 1)

# --- $20 efficiency: quality delivered per output token, indexed to best = 100 ---
# Output tokens are the fair cross-model effort proxy (cache-heavy totals mislead).
# A raw quality/output ratio is degenerate: gemma4-agent-26b emitted 276 output tokens
# for a tier-B plan and would score "most efficient in field" purely for writing almost
# nothing. So the denominator gets a FLOOR of 15k tokens - roughly the least output any
# genuinely complete plan in this field used (glm-5.2 hit tier S on 19.5k). Below the
# floor a model is not efficient, it is incomplete, and gets no ratio bonus.
OUT_FLOOR = 15_000
for m in models:
    denom = max(m["out"], OUT_FLOOR)
    m["qpt"] = m["quality"] / denom
    m["thin"] = m["out"] < OUT_FLOOR      # flag: output too small to be a complete plan
best_qpt = max(m["qpt"] for m in models) or 1
for m in models:
    m["eff"] = round(100 * m["qpt"] / best_qpt, 1)

models.sort(key=lambda m: m["rank"])
# Default view: the 6 top-ranked plans PLUS the efficiency champions, so the opening
# screen shows both "who won" and "who won cheaply" - the two headline stories.
EFF_PICKS = {"minimax-m3", "glm-5.2-cloud", "qwen3.5-397b", "fable-5 v1"}
DEFAULT_ON = {m["plan"] for m in models[:6]} | EFF_PICKS

payload = {
    "cats": [c[0] for c in CATS] + [c[0] for c in NEW_CATS],
    "models": models,
    "defaultOn": sorted(DEFAULT_ON),
}

TPL = """<!DOCTYPE html>
<html lang="en" data-theme="dark">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Model bake-off — yt-player-scheduler planning task</title>
<style>
  :root{
    --surface-1:#1a1a19; --page:#0d0d0d;
    --ink:#ffffff; --ink-2:#c3c2b7; --muted:#898781;
    --grid:#2c2c2a; --axis:#383835; --ring:rgba(255,255,255,0.10);
    --r1:#184f95; --r2:#256abf; --r3:#3987e5; --r4:#86b6ef;
    --good:#0ca30c; --warning:#fab219; --serious:#ec835a; --critical:#d03b3b;
    --tierS:#3987e5; --tierA:#199e70; --tierB:#c98500; --tierC:#d95926; --tierF:#e66767;
  }
  *{box-sizing:border-box}
  html,body{margin:0;background:var(--page);color:var(--ink);
    font-family:system-ui,-apple-system,"Segoe UI",sans-serif}
  .wrap{padding:20px 24px 40px;max-width:1900px;margin:0 auto}
  h1{font-size:20px;margin:0 0 4px;font-weight:650;letter-spacing:-.01em}
  .sub{color:var(--ink-2);font-size:13px;margin:0 0 18px}
  .sub code{color:var(--r4)}
  .controls{background:var(--surface-1);border:1px solid var(--ring);border-radius:10px;
    padding:12px 14px;margin-bottom:16px}
  .controls h2{font-size:11px;text-transform:uppercase;letter-spacing:.08em;
    color:var(--muted);margin:0 0 10px;font-weight:600}
  .chips{display:flex;flex-wrap:wrap;gap:6px;align-items:center}
  .chip{background:transparent;color:var(--ink-2);border:1px solid var(--axis);
    border-radius:999px;padding:5px 11px;font-size:12px;cursor:pointer;
    font-family:inherit;transition:.12s;white-space:nowrap;display:inline-flex;
    align-items:center;gap:6px}
  .chip:hover{border-color:var(--r3);color:var(--ink)}
  .chip[aria-pressed="true"]{background:var(--r1);border-color:var(--r3);color:#fff}
  .chip .tdot{width:7px;height:7px;border-radius:50%;flex:0 0 auto}
  .chip .ret{font-size:10px;color:var(--warning)}
  .hcap{font-size:11px;color:var(--critical)}
  .pair{font-size:11px;color:var(--r4)}
  .bulk{margin-left:auto;display:flex;gap:6px}
  .bulk .chip{border-style:dashed}
  .scroll{overflow:auto;border:1px solid var(--ring);border-radius:10px;
    background:var(--surface-1);max-height:76vh}
  table{border-collapse:separate;border-spacing:0;width:100%;font-size:13px}
  th,td{padding:9px 12px;text-align:left;border-bottom:1px solid var(--grid);
    white-space:nowrap}
  thead th{position:sticky;top:0;z-index:3;background:var(--surface-1);
    border-bottom:1px solid var(--axis);vertical-align:bottom;padding-bottom:11px}
  tbody th,thead th:first-child{position:sticky;left:0;z-index:2;
    background:var(--surface-1);border-right:1px solid var(--axis)}
  thead th:first-child{z-index:4}
  tbody th{font-weight:500;color:var(--ink-2);font-size:12px}
  .mname{font-weight:650;font-size:13px;color:var(--ink);display:block}
  .mmeta{font-size:11px;color:var(--muted);font-weight:400;display:block;margin-top:3px;
    font-variant-numeric:tabular-nums}
  .tier{display:inline-block;font-size:10px;font-weight:700;padding:1px 6px;
    border-radius:4px;color:#0d0d0d;margin-right:5px}
  .cell{display:flex;align-items:center;gap:9px}
  .meter{display:flex;gap:2px;flex:0 0 auto}
  .seg{width:9px;height:14px;border-radius:2px;background:var(--axis)}
  .seg.on{background:var(--r3)}
  .s0 .seg.on{background:var(--critical)} .s1 .seg.on{background:var(--serious)}
  .s2 .seg.on{background:var(--warning)}  .s3 .seg.on{background:var(--r3)}
  .s4 .seg.on{background:var(--good)}
  .ctext{font-size:12px;color:var(--ink-2);overflow:hidden;text-overflow:ellipsis}
  tbody tr:hover td,tbody tr:hover th{background:#232322}
  tr.spacer td{height:6px;padding:0;border:none;background:transparent}
  tr.section th{font-size:10px;text-transform:uppercase;letter-spacing:.08em;
    color:var(--muted);font-weight:700;border-bottom:1px solid var(--axis);
    padding-top:14px}
  tr.section td{border-bottom:1px solid var(--axis)}
  .num{font-variant-numeric:tabular-nums;font-size:13px;color:var(--ink)}
  .effbar{height:6px;border-radius:3px;background:var(--axis);width:70px;
    overflow:hidden;flex:0 0 auto}
  .effbar i{display:block;height:100%;border-radius:3px;background:var(--r4)}
  .legend{display:flex;flex-wrap:wrap;gap:16px;margin-top:14px;font-size:11px;
    color:var(--muted);align-items:center}
  .legend .li{display:flex;align-items:center;gap:6px}
  .legend .seg{width:8px;height:11px}
  .foot{margin-top:14px;font-size:11px;color:var(--muted);line-height:1.6;max-width:1100px}
  .hidden{display:none}
  [data-tip]{cursor:help}
</style>
</head>
<body>
<div class="wrap viz-root">
  <h1>Model bake-off — one identical planning task</h1>
  <p class="sub">__NMODELS__ runs, same prompt: extend the Go <code>yt-player-scheduler</code> with a download web UI.
     Each category rated 0–4; hover any cell for the original finding.</p>

  <div class="controls">
    <h2>Models — click to show / hide</h2>
    <div class="chips" id="chips"></div>
  </div>

  <div class="scroll">
    <table id="tbl">
      <thead><tr id="hrow"></tr></thead>
      <tbody id="body"></tbody>
    </table>
  </div>

  <div class="legend" id="legend">
    <span class="li"><b style="color:var(--ink-2)">Rating:</b></span>
    <span class="li s0"><span class="meter"><span class="seg on"></span></span> 0 — wrong / missing</span>
    <span class="li s1"><span class="meter"><span class="seg on"></span><span class="seg on"></span></span> 1 — weak</span>
    <span class="li s2"><span class="meter"><span class="seg on"></span><span class="seg on"></span><span class="seg on"></span></span> 2 — partial</span>
    <span class="li s3"><span class="meter"><span class="seg on"></span><span class="seg on"></span><span class="seg on"></span><span class="seg on"></span></span> 3 — good</span>
    <span class="li s4"><span class="meter"><span class="seg on"></span><span class="seg on"></span><span class="seg on"></span><span class="seg on"></span><span class="seg on"></span></span> 4 — best in field</span>
  </div>

  <p class="foot">
    <b>$20 efficiency</b> = plan quality delivered per <i>output</i> token, indexed to the field's best = 100.
    Output tokens are the fair cross-model effort proxy — Anthropic/GPT totals are mostly cheap cache-reads, so
    raw <i>total</i> tokens overstate real work. High efficiency = a good plan without burning generation budget,
    which is what a $20 budget actually buys.
    <br><b>⚠ retired</b> = model has since been retired from Ollama Cloud (waves 2026-07-15 / 07-31):
    its result stands as history but it can't be re-run or demoed live.
    <br><b>The last four rows are hand-scored</b> from a full read of all 25 plans, not keyword-derived
    like the first 13. They were added because the original 13 correlated heavily (one latent
    "did it go and check" factor) and missed real strengths — <code>opus-4-8</code> ranked below
    <code>opus-4-7</code> purely because its verified-facts block and commit/subagent plan had nowhere to
    score. <b>Input/command safety is the most independent axis in the table</b> (mean |r| = 0.34
    against the other 13): <code>glm-5.3</code> scores 4 on it, both Opus plans score 0.
    <br><b>⛒ / ⇄ fable-5 v1 and v2 are the same model on the same prompt, kept deliberately.</b>
    v1 (⛒) hit a Bash-tool outage that blocked <code>git show</code> and ssh, so it
    reconstructed the target commit from the reflog and scored 2/4 on grounding. v2 (⇄)
    verified everything — ssh'd the Pi, read the real <code>yt.sh</code>, checked MPD codecs — and used
    <b>42% fewer requests and 37% fewer tokens while generating 25% more output</b>. Same weights,
    different environment: a reminder that a "model score" is partly a score of the harness it ran in.
  </p>
</div>

<script>
const DATA = __PAYLOAD__;
const TIERC = {S:'--tierS',A:'--tierA',B:'--tierB',C:'--tierC',F:'--tierF'};
const on = new Set(DATA.defaultOn);
const esc = s => String(s).replace(/[&<>"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));

function meter(score){
  let h = '<span class="meter">';
  for(let i=0;i<5;i++) h += '<span class="seg'+(i<=score?' on':'')+'"></span>';
  return h+'</span>';
}
function shown(){ return DATA.models.filter(m=>on.has(m.plan)); }

function chips(){
  document.getElementById('chips').innerHTML =
    DATA.models.map(m=>{
      const c = 'var('+(TIERC[m.tier]||'--muted')+')';
      return '<button class="chip" role="button" aria-pressed="'+on.has(m.plan)+'" data-p="'+esc(m.plan)+'">'
        + '<span class="tdot" style="background:'+c+'"></span>'
        + esc(m.plan)
        + (m.retired?' <span class="ret">⚠</span>':'')
        + (m.handicapped?' <span class="hcap" title="environment-handicapped: tool outage during the run">⛒</span>':'')
        + (m.paired&&!m.handicapped?' <span class="pair" title="paired re-run of the same model+prompt">⇄</span>':'')
        + '</button>';
    }).join('')
    + '<span class="bulk">'
    + '<button class="chip" data-bulk="top8">top 8</button>'
    + '<button class="chip" data-bulk="all">all</button>'
    + '<button class="chip" data-bulk="none">none</button></span>';
}

function render(){
  const ms = shown();
  document.getElementById('hrow').innerHTML =
    '<th>Category</th>' + ms.map(m=>{
      const c='var('+(TIERC[m.tier]||'--muted')+')';
      return '<th><span class="mname">'+esc(m.plan)
        +(m.retired?' <span class="ret" data-tip="retired from Ollama Cloud">⚠</span>':'')
        +(m.handicapped?' <span class="hcap" data-tip="environment-handicapped: Bash-tool outage blocked git show/ssh, so grounding was reflog-reconstructed">⛒</span>':'')
        +(m.paired&&!m.handicapped?' <span class="pair" data-tip="clean re-run of the same model on the same prompt">⇄</span>':'')
        +'</span>'
        +'<span class="mmeta"><span class="tier" style="background:'+c+'">'+esc(m.tier)+'</span>'
        +'#'+m.rank+' · '+(m.out/1000).toFixed(1)+'k out</span></th>';
    }).join('');

  let rows = '<tr class="section"><th>Headline</th>'+ms.map(()=>'<td></td>').join('')+'</tr>';

  rows += '<tr><th>Plan quality (0–100)</th>' + ms.map(m=>
    '<td><div class="cell"><span class="effbar"><i style="width:'+m.quality+'%"></i></span>'
    +'<span class="num">'+m.quality+'</span></div></td>').join('') + '</tr>';

  rows += '<tr><th>$20 efficiency (index)</th>' + ms.map(m=>
    '<td><div class="cell"><span class="effbar"><i style="width:'+Math.max(2,m.eff)+'%"></i></span>'
    +'<span class="num">'+m.eff+'</span></div></td>').join('') + '</tr>';

  rows += '<tr><th>Output tokens</th>' + ms.map(m=>
    '<td><span class="num">'+m.out.toLocaleString()+'</span></td>').join('') + '</tr>';

  rows += '<tr class="section"><th>Scored categories</th>'+ms.map(()=>'<td></td>').join('')+'</tr>';

  DATA.cats.forEach(cat=>{
    rows += '<tr><th>'+esc(cat)+'</th>' + ms.map(m=>{
      const c = m.cells[cat];
      return '<td><div class="cell s'+c.s+'" data-tip="'+esc(c.raw)+'" title="'+esc(c.raw)+'">'
        + meter(c.s) + '<span class="ctext">'+esc(c.t)+'</span></div></td>';
    }).join('') + '</tr>';
  });

  document.getElementById('body').innerHTML = ms.length ? rows :
    '<tr><td style="color:var(--muted);padding:26px">No models selected — pick some above.</td></tr>';
}

document.getElementById('chips').addEventListener('click', e=>{
  const b = e.target.closest('button'); if(!b) return;
  if(b.dataset.bulk){
    on.clear();
    if(b.dataset.bulk==='all') DATA.models.forEach(m=>on.add(m.plan));
    if(b.dataset.bulk==='top8') DATA.defaultOn.forEach(p=>on.add(p));
  } else {
    const p = b.dataset.p;
    on.has(p) ? on.delete(p) : on.add(p);
  }
  chips(); render();
});

chips(); render();
</script>
</body>
</html>
"""

with open(OUT, "w") as f:
    f.write(TPL.replace("__PAYLOAD__", json.dumps(payload, separators=(",", ":")))
             .replace("__NMODELS__", str(len(models))))
print("wrote", OUT)
print("models:", len(models), "| categories:", len(CATS))
print("top efficiency:", sorted(((m['eff'], m['plan']) for m in models), reverse=True)[:5])
