# OCR Server Resourcing & the Local-LLM Decision

**Status:** research input for Feature 004 (Arabic handwriting OCR)
**Date:** 2026-09-03
**Author:** generated from a measured local-VLM evaluation run on this machine
**Constraint that frames everything below:** the project constitution forbids any
attachment or derived text leaving the machine. **Cloud OCR (Google Vision, Azure
Document Intelligence, AWS Textract, any hosted VLM API) is therefore out of scope by
rule, not by preference.** The only lawful options are (a) local Tesseract, (b) a local
self-hosted VLM, or (c) human transcription.

---

## 1. TL;DR recommendation

1. **Do NOT put a local LLM on the critical path for legal fields.** Measured accuracy on a
   real, degraded Egyptian deed was **~15–20 % exact-word accuracy, and 0 % on the fields
   that matter** (names, numbers, the registry header). That is not fit for unattended
   extraction into a property record.
2. **Keep Feature 003 (Tesseract `ara+eng` + auto-rotation) for typed text.** It is the
   right tool for the typed document body and needs only a small CPU server.
3. **Handwriting → human transcription for now.** Optionally run a local Arabic-OCR VLM
   (Qari-OCR 0.4.0) as a *draft/assist* a human corrects — but only fund the hardware for
   it after a pilot proves it actually saves human time.
4. **Provision the cheap CPU server now (Scenario A). Only buy the GPU box (Scenario B) if
   the assist earns its cost.**

---

## 2. What was measured (why the recommendation is what it is)

Seven local vision-language model configurations were tested on the same page: a photo of a
photocopy of an Egyptian property deed — rotated 90°, typed Arabic body + handwritten margin
notes, stamps, signatures. Protocol: Tesseract OSD auto-rotate → 4 horizontal tiles →
identical prompt and sampling params (`temperature 0.2, top_p 0.9, repeat_penalty 1.2`).
Scoring was against a human-provided ground-truth fragment using edit distance.

| Model (runtime) | Terminates cleanly? | Exact word accuracy | Notes |
|---|---|---|---|
| **Qari-OCR 0.4.0-VL-4B** (Ollama, Q8) | 3 of 4 tiles | **6–19 %** (best run 19 %) | Best Arabic reader; still 0 % on proper nouns & header |
| qwen2.5-VL 7B (Ollama) | 4 of 4 | — | **Fabricated a fake document** on the ground-truth tile, with a clean stop signal |
| Qari-OCR v0.3-VL-2B (MLX) | 2 of 4 | low | Uniquely recovered `مطران أسيوط`, `رئيس الملائكة ميخائيل`; loops on dense tiles |
| Qwen3-VL 4B (MLX, 4-bit & 8-bit) | 0 of 4 | — | Reads correctly for one pass, then loops forever |
| Gemma-3 4B base (MLX) | 0 of 4 | — | Loops + fabricates; base model, not a bad fine-tune |
| InternVL3-2B (MLX) | 2 of 4 | — | Describes the image instead of transcribing |

**Key facts that drive the decision:**

- **Preprocessing did not help.** 2× upscaling, grayscale+sharpen, and OCR-specific prompts
  all scored *equal or worse* than the plain baseline. The ceiling is the model + the
  degraded scan, not the pipeline.
- **The clean-termination signal is not a correctness signal.** qwen2.5-VL produced a
  confident, well-formed, completely fabricated document and reported `done_reason=stop`.
  Confident + plausible + wrong is the worst failure mode for a legal record.
- **Names, numbers, and the document-type header are the weakest output everywhere** — and
  those are exactly the fields a property record depends on.
- **The bottleneck is the source image** (a photo of a photocopy). No local model cleared it.

Raw per-tile outputs from the evaluation were saved during the run; this document is the
distilled conclusion.

---

## 3. The decision: run a local LLM, or not?

### Case AGAINST a local LLM (recommended default)
- Accuracy on real, degraded deeds is far below what a legal record requires.
- It cannot be trusted unattended, so a human must verify every field anyway — which caps
  the time it can save.
- It adds GPU/hardware cost, model-update maintenance, and a fabrication-risk surface.
- Tesseract already handles the typed body, which is the majority of extractable text.

### Case FOR a local LLM (only as a human-verified assist)
- The Arabic specialist (Qari-OCR 0.4.0) does recover *some* real structure typed Tesseract
  misses on messy layouts, and reads a portion of the handwritten body.
- As a **draft that a human corrects**, it may reduce transcription time for high-volume
  intake — *if* a pilot proves it. It must never auto-populate a field unreviewed.
- It keeps all data on-machine, satisfying the constitution, where no cloud API can.

### Verdict
**Local LLM = optional assist, never the source of truth.** Ship Tesseract now; pilot the
VLM assist before funding hardware for it.

---

## 4. Server resourcing

### Scenario A — Tesseract only (no local LLM) — RECOMMENDED to provision now
Feature 003 as designed: Tesseract `ara+eng` + Poppler + Tesseract OSD auto-rotation,
invoked over pipes (no plaintext to disk).

| Resource | Minimum | Recommended |
|---|---|---|
| CPU | 2 vCPU | 4 vCPU (parallel OCR jobs) |
| RAM | 4 GB | 8 GB |
| GPU | none | none |
| Disk | 10 GB | 20 GB |
| Throughput | ~1–3 s/page (CPU) | — |

- Cheapest tier; runs on any small Linux VM or a modest on-prem box.
- Tesseract is CPU-bound and does not benefit from a GPU.
- **Note:** `tesseract-ocr` with the `ara`, `eng`, and `osd` language data is a hard
  dependency here (a few tens of MB), plus `poppler-utils` for PDF rasterization.

### Scenario B — Add local VLM assist (Feature 004, human-verified)
The model must fit in memory alongside its vision encoder and KV cache.
Qari-OCR 0.4.0: ~5 GB (Q8) or ~2.8 GB (Q4) weights + ~0.8 GB vision projector + context →
budget **~8–10 GB of GPU/unified memory**.

**Option B1 — NVIDIA GPU server (Linux; Ollama / llama.cpp) — best price/perf for a server**

| Resource | Minimum | Recommended |
|---|---|---|
| GPU VRAM | 12 GB (Q4 only) — e.g. RTX 4070 / A2000-12G | 16–24 GB — NVIDIA **L4 24 GB**, RTX 4000 Ada 20 GB, or A10 24 GB |
| CPU | 4 vCPU | 8 vCPU |
| RAM | 16 GB | 32 GB |
| Disk | 40 GB | 60 GB (models + scratch) |
| Speed | ~30–90 s/tile (CPU fallback) | ~2–8 s/tile on L4/A10 |

- **L4 24 GB** is the sweet spot: enough VRAM for Q8 + comfortable context, low power,
  data-center supported.
- CPU-only inference works but is slow (30–90 s/tile) — acceptable only for very low volume.

**Option B2 — Apple Silicon (Mac mini / Mac Studio; MLX runtime)**

| Resource | Minimum | Recommended |
|---|---|---|
| Chip | M4 / M4 Pro | M4 Pro / M4 Max |
| Unified memory | 24 GB | 32–64 GB |
| Disk | 256 GB | 512 GB |
| Speed | small models 4–5× faster than CPU; ~5–15 s/tile typical | — |

- In this evaluation, MLX on Apple Silicon was markedly faster than CPU for small VLMs and
  has excellent perf-per-watt. Trade-off: running macOS as a headless server is less
  standard for ops than a Linux GPU host.
- Good fit **only if the team already operates on Apple Silicon**; otherwise prefer B1.

### What NOT to buy
- **No cloud OCR / hosted VLM API** — forbidden by the constitution (data must stay local).
- **No high-end multi-GPU rig** — a single 4B–7B OCR model does not need it, and throwing
  more compute at it will not raise accuracy on this input.

---

## 5. Recommended path

1. **Now:** provision **Scenario A** (small CPU server). Ship Feature 003 for typed text.
   Handwriting is transcribed by a human, every field verified.
2. **Pilot (optional):** stand up **Qari-OCR 0.4.0** on a single modest GPU (rent an L4 by
   the hour, or reuse an Apple Silicon dev machine) and measure whether the *assist* reduces
   human transcription time on a real batch. Gate every output behind human review; discard
   any tile whose generation hit the length cap; never trust the stop signal as correctness.
3. **Decide on B only from pilot data.** If the assist demonstrably saves time, provision
   **Scenario B1 (NVIDIA L4)**. If not, stay on Scenario A and keep handwriting fully human.

**Cross-cutting rules for any VLM use (from the measured failures):**
- Auto-rotate with Tesseract OSD first — confirmed necessary.
- Feed horizontal tiles, never the whole page — whole-page input triggers fabrication loops.
- Never auto-populate a legal field from model output; human verification is mandatory.
- Treat `finish_reason=length` as "discard"; do **not** treat `stop` as "correct".
