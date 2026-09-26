# Benchmark results

Generated from `runs.jsonl` by `record.py` — do not edit.

| when | agent | model | dataset | trials | mean reward | errors | input tok | output tok | cost $ | commit | purpose |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 2026-09-26 11:30 | isekai | vllm/fake/vllm-coder | fake/vllm-coder__adhoc | 1 | 0.0000 | 0 | 2700 | 80 | — | a815eb6+dirty | smoke: fake vLLM (wrong) |
| 2026-09-26 11:30 | isekai | vllm/fake/vllm-coder | fake/vllm-coder__adhoc | 1 | 1.0000 | 0 | 2700 | 80 | — | a815eb6+dirty | smoke: fake vLLM (right) |
| 2026-09-26 10:43 | nop | — | local:tasks/hello-file | 1 | 0.0000 | 0 | — | — | — | e6ddce8 | task validation (reference solution must pass, nop must fail) |
| 2026-09-26 10:43 | oracle | — | local:tasks/hello-file | 1 | 1.0000 | 0 | — | — | — | e6ddce8 | task validation (reference solution must pass, nop must fail) |
