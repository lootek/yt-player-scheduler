Plan implementation as in ai/opus-4-8.md with the followind adjustments:

- crash handling/isolation as in ai/opus-4-7.md
- docker mount change as mpd gotcha as in ai/opus-4-7.md
- security as in ai/kimi-k2.7-code-cloud.md (injection protection), timeout, race caveat
- history.jsonl as in ai/minimax-m3.md
- XSS protection as in ai/minimax-m3.md
- 3 checkboxes
- no per-job subdir (there should be a global subdir configurable for the whole web UI-driven new feature, other than the one for cron)
- make sure to retain naming fidelity with yt.sh

Ignore the other plans/files from ai/
