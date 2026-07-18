# Krea 2 Turbo Pure-Go Image Gen Spike — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate an end-to-end image from a text prompt fully in pure Go: Krea 2 Turbo (12B `qwen_image` MMDiT, GGUF Q4_K_M) running in the born fork on 64 GB Apple Silicon, via `cmd/imagegen`.

**Architecture:** Five parity-gated ports in dependency order — generic GGUF loader → Qwen2.5-VL text encoder (text path, reusing born's LLaMA stack) → Qwen-Image MMDiT → VAE decoder → flow-matching Euler sampler — transliterated from stable-diffusion.cpp with diffusers as readable cross-reference. A Python harness loads the *same dequantized GGUF weights* into diffusers modules to dump parity fixtures, so any divergence is a born-side bug.

**Tech Stack:** Go (born fork, CPU backend, zero CGO), Python 3.11+ venv (torch, diffusers, transformers, gguf) for fixtures only, stable-diffusion.cpp (Metal) as e2e oracle.

**Spec:** `docs/superpowers/specs/2026-07-18-krea2-image-gen-spike-design.md`

## Phase-0 Amendments (2026-07-18, discovered facts — supersede task wording below)

Pinned in `<born>/qwenimage/reference/ORACLE.md` from sd.cpp source (`docs/krea2.md`, `src/model/diffusion/krea2.hpp`, `src/conditioning/conditioner.hpp`):

- **Text encoder is Qwen3-VL 4B Instruct** (36 layers, hidden 2560), not Qwen2.5-VL 7B. Tasks 5–6 read "Qwen3-VL 4B" wherever they say "Qwen2.5-VL"; tokenizer files come from `Qwen/Qwen3-VL-4B-Instruct`. Conditioning = hidden states from layers {2,5,8,11,14,17,20,23,26,29,32,35}, first 34 tokens dropped, fixed system-prompt template (see ORACLE.md).
- **The DiT is Krea2's own single-stream architecture** (28 blocks, features 6144, GQA 48/12, shared modulation + per-block learned offset, sigmoid-gated attention, txtfusion transformer over the 12-layer text stack) — not qwen_image dual-stream. sd.cpp `krea2.hpp` (783 lines) is the complete blueprint; Task 7's OPS.md derives from it. diffusers cross-reference only if a Krea2 pipeline exists there; otherwise the harness transliterates krea2.hpp into PyTorch directly (which then doubles as the fixture generator).
- **Scheduler: FLUX_FLOW_PRED, constant flow shift 1.15** (not resolution-dynamic) — simplifies Task 10's `Sigmas` to the static shift formula.
- **VAE is literally Wan 2.1 VAE** (`wan_2.1_vae.safetensors`, 254 MB).
- DiT GGUF source: `realrebelai/KREA-2_GGUFs` TURBO/Q4_K_M (7.22 GB), per sd.cpp docs (instead of Abiray repo).
- sd.cpp binary is `sd-cli`; flags: `--diffusion-model <dit.gguf> --llm <qwen3vl.gguf> --vae <wan_vae.safetensors>`.

## Global Constraints

- Work happens in a born fork clone at `/Users/arturoaquino/Documents/manifold-tmp/users/0/projects/3eb68163-63b9-4957-bb56-415b63ceb5c2/born` (sibling of `manifold/`), branch **`feat/krea2-image`** off **`feat/supertonic-tts`**, remote `github.com/intelligencedev/born`. Push after every completed task (the remote is the preservation mechanism — the TTS spike lost its clone once).
- Model files live in `~/.cache/manifold/krea2-models/` (precedent: `moonshine-models`). Never committed.
- Python harness + fixture scripts live in-fork at `qwenimage/reference/`; fixtures written to `qwenimage/reference/fixtures/` (gitignored; regenerable).
- Fixture format: raw little-endian float32 `.bin` + JSON sidecar `{"shape": [...], "dtype": "float32"}`. One pair per tensor, named `<component>.<tensor>.bin/.json`.
- Strict loading everywhere: unknown/missing GGUF tensor names, unmapped ops, or shape mismatches are hard errors. Never silently skip (the TTS silent-skip trap).
- Parity gates (F32 compute both sides, identical dequantized weights): per-op/per-block maxAbs ≤ 1e-4; full-component outputs maxAbs ≤ 1e-3 (60-layer accumulation). Record ACTUAL numbers in STATUS.md; investigate anything near the gate rather than sliding past it.
- Tiny-config parity for iteration: diffusers/transformers classes accept small configs (2 layers, small dims) — use random-weight tiny models for fast red/green cycles; run full-weight parity once per gate.
- Go: gofmt clean, `go vet` clean, unit tests table-driven and `-race` where practical. `go fmt` every touched file before commit.
- Supertonic + Moonshine test suites must stay green on the branch (regression gate).
- Spike baseline: 512×512, 8 steps, CFG 0.0, seed fixed to 42 for determinism tests.
- Do NOT modify the manifold repo in this plan (except plan-checkbox updates and STATUS notes) — Manifold `/image` wiring is out of scope per spec.

---

## Phase 0 — Workspace & Oracles

### Task 1: Fork workspace on `feat/krea2-image`

**Files:**
- Create: `<born>/qwenimage/` (package dir, empty doc.go)
- Create: `<born>/qwenimage/reference/.gitignore` (`fixtures/`, `*.bin`)

**Interfaces:**
- Produces: a building fork clone on branch `feat/krea2-image`; package path `github.com/intelligencedev/born/qwenimage`.

- [ ] **Step 1: Clone and branch**

```bash
cd /Users/arturoaquino/Documents/manifold-tmp/users/0/projects/3eb68163-63b9-4957-bb56-415b63ceb5c2
git clone https://github.com/intelligencedev/born
cd born && git checkout feat/supertonic-tts && git checkout -b feat/krea2-image
```

Expected: branch `feat/krea2-image` at the tip that manifold's go.mod pins (`f7f8a33` or later).

- [ ] **Step 2: Baseline test run**

```bash
go build ./... && go test ./internal/... ./supertonic/... ./moonshine/... 2>&1 | tail -20
```

Expected: build OK; unit suites PASS (parity tests may SKIP without local model assets — note which skip, that's the pre-existing state).

- [ ] **Step 3: Scaffold package + commit**

`qwenimage/doc.go`:

```go
// Package qwenimage implements text-to-image generation for Qwen-Image
// architecture models (Krea 2 Turbo) from GGUF weights, pure Go.
package qwenimage
```

```bash
git add qwenimage && git commit -m "qwenimage: scaffold package for Krea 2 Turbo spike" && git push -u origin feat/krea2-image
```

### Task 2: stable-diffusion.cpp oracle + model downloads

**Files:**
- Create: `<born>/qwenimage/reference/ORACLE.md` (exact file inventory, exact sd.cpp command, reference image)

**Interfaces:**
- Produces: `~/.cache/manifold/krea2-models/` populated (DiT Q4_K_M GGUF, Qwen2.5-VL text-encoder weights, VAE weights, tokenizer files); a known-good sd.cpp command generating `reference/oracle-512-seed42.png`.

- [ ] **Step 1: Build sd.cpp with Metal**

```bash
cd /Users/arturoaquino/Documents/manifold-tmp/users/0/projects/3eb68163-63b9-4957-bb56-415b63ceb5c2
git clone --recursive https://github.com/leejet/stable-diffusion.cpp
cmake -S stable-diffusion.cpp -B stable-diffusion.cpp/build -DSD_METAL=ON -DCMAKE_BUILD_TYPE=Release
cmake --build stable-diffusion.cpp/build -j
```

Expected: `stable-diffusion.cpp/build/bin/sd` exists. Then read `stable-diffusion.cpp/docs/` for the qwen_image doc page and `./sd --help` to get the exact flags for diffusion-model / text-encoder / VAE.

- [ ] **Step 2: Download models (ASK USER BEFORE DOWNLOADING — multi-GB)**

DiT: `huggingface-cli download Abiray/Krea-2-Turbo-GGUF Krea-2-Turbo-Q4_K_M.gguf --local-dir ~/.cache/manifold/krea2-models/` (~7.5 GB; confirm exact filename from repo listing). Companion text-encoder GGUF + VAE: per the sd.cpp qwen_image doc discovered in Step 1 (expected: a Qwen2.5-VL-7B text-encoder GGUF, e.g. city96/Comfy-Org repack, and `qwen_image_vae.safetensors`). Tokenizer: `huggingface-cli download Qwen/Qwen2.5-VL-7B-Instruct tokenizer.json tokenizer_config.json --local-dir ~/.cache/manifold/krea2-models/tokenizer/`. Record every file+SHA+source URL in `ORACLE.md`.

- [ ] **Step 3: Generate reference image**

Run the sd.cpp command (exact flags from Step 1) with `-p "a red fox sitting in fresh snow, golden hour, photorealistic" --steps 8 --cfg-scale 0.0 -W 512 -H 512 -s 42`. Expected: a recognizable fox → save as `qwenimage/reference/oracle-512-seed42.png` (commit — it's small). Record wall time + peak memory in ORACLE.md.

- [ ] **Step 4: Commit + push**

```bash
git add qwenimage/reference && git commit -m "qwenimage: oracle inventory + sd.cpp reference image" && git push
```

### Task 3: Python parity harness, self-validated

**Files:**
- Create: `<born>/qwenimage/reference/requirements.txt` (`torch`, `diffusers`, `transformers`, `gguf`, `safetensors`, `numpy`, `accelerate`, `sentencepiece`, `pillow`)
- Create: `<born>/qwenimage/reference/harness.py`
- Create: `<born>/qwenimage/reference/dump.py` (fixture writer)

**Interfaces:**
- Produces: `python harness.py e2e --out harness-512-seed42.png` (self-validation); `python harness.py dump-<component>` subcommands used by later tasks; fixture writer `dump(name, tensor)` → `.bin` + `.json` per Global Constraints.

- [ ] **Step 1: venv + deps**

```bash
cd <born>/qwenimage/reference && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt
```

- [ ] **Step 2: Write `dump.py`**

```python
import json, pathlib
import numpy as np, torch
FIX = pathlib.Path(__file__).parent / "fixtures"
def dump(name: str, t):
    FIX.mkdir(exist_ok=True)
    if isinstance(t, torch.Tensor):
        t = t.detach().to("cpu", torch.float32).numpy()
    a = np.ascontiguousarray(t, dtype=np.float32)
    (FIX / f"{name}.bin").write_bytes(a.tobytes())
    (FIX / f"{name}.json").write_text(json.dumps({"shape": list(a.shape), "dtype": "float32"}))
```

- [ ] **Step 3: Write `harness.py` e2e path**

Load the *same* GGUF DiT via diffusers GGUF support, full pipeline in F32 on CPU (MPS fallback allowed for self-validation only):

```python
import torch
from diffusers import GGUFQuantizationConfig
# Try Krea2 classes first; fall back to QwenImage classes (same architecture family).
from diffusers import QwenImageTransformer2DModel, QwenImagePipeline  # or Krea2Pipeline if present
MODELS = "~/.cache/manifold/krea2-models"
transformer = QwenImageTransformer2DModel.from_single_file(
    f"{MODELS}/Krea-2-Turbo-Q4_K_M.gguf",
    quantization_config=GGUFQuantizationConfig(compute_dtype=torch.float32))
# text_encoder + vae + scheduler from krea/Krea-2-Turbo (or Qwen/Qwen-Image) HF repo
# pipe = <Pipeline>.from_pretrained(..., transformer=transformer, torch_dtype=torch.float32)
# image = pipe(prompt, width=512, height=512, num_inference_steps=8,
#              true_cfg_scale=0.0, generator=torch.Generator("cpu").manual_seed(42)).images[0]
```

The commented lines are resolved against whichever pipeline class diffusers ships for this model (check `diffusers` docs/source for `Krea2` first). Add subcommands as thin wrappers: `dump-text-encoder`, `dump-dit-step`, `dump-vae`, `dump-scheduler`, `dump-gguf-tensors`, each running one component on fixed inputs and calling `dump()`.

- [ ] **Step 4: Self-validate**

Run: `.venv/bin/python harness.py e2e --out harness-512-seed42.png`
Expected: a recognizable fox comparable to the sd.cpp oracle. **Until this passes, no fixture from the harness is trusted.** Record wall time in ORACLE.md.

- [ ] **Step 5: Commit + push**

```bash
git add qwenimage/reference && git commit -m "qwenimage: python parity harness (self-validated e2e)" && git push
```

---

## Phase 1 — Generic GGUF loader

### Task 4: `gguf` tensor enumeration + on-demand dequant

**Files:**
- Create/Modify: born's existing GGUF package (locate via `grep -r "Q4_K" --include=*.go`; extend in place, keeping the LLaMA path working)
- Test: alongside, `*_test.go`

**Interfaces:**
- Produces (exact API later tasks consume):

```go
package gguf // or born's existing package name

func Open(path string) (*File, error)
func (f *File) Close() error
func (f *File) TensorNames() []string
func (f *File) Meta(key string) (any, bool)          // arch, dims, etc.
func (f *File) TensorInfo(name string) (TensorInfo, bool) // Shape []int, Quant string
func (f *File) LoadF32(name string) ([]float32, []int, error) // dequantized, shape
```

- [ ] **Step 1: Failing test — enumerate + dequant vs Python fixture**

Harness: `harness.py dump-gguf-tensors` picks 6 tensors from the DiT GGUF spanning quant types present (Q4_K, Q6_K, F32 norm weights…), dumps each dequantized plus a `gguf-manifest.json` (name→shape,quant). Go test (skips if models absent, TTS convention):

```go
func TestGGUFDequantParity(t *testing.T) {
    f := mustOpen(t, kreaGGUFPath()) // skips if absent
    for _, name := range manifestNames(t) {
        got, shape, err := f.LoadF32(name)
        require.NoError(t, err)
        want, wantShape := loadFixture(t, "gguf."+name)
        require.Equal(t, wantShape, shape)
        require.Less(t, maxAbsDiff(got, want), 1e-6) // dequant is deterministic math
    }
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./<pkg> -run TestGGUFDequantParity -v` fails (API missing).

- [ ] **Step 3: Implement** — generalize the LLaMA-shaped reader: parse GGUF v3 header generically (it already does), expose the tensor table, route each quant block type to the existing K-quant dequant kernels. Strict: `LoadF32` on unknown quant type returns error naming it.

- [ ] **Step 4: Verify pass + LLaMA regression** — target test PASS; existing GGUF/LLaMA tests PASS.

- [ ] **Step 5: Commit + push** — `git commit -m "gguf: generic tensor enumeration + on-demand K-quant dequant"`

---

## Phase 2 — Text encoder

### Task 5: Qwen2 byte-level BPE tokenizer

**Files:**
- Create: `<born>/qwenimage/tokenizer.go`, `tokenizer_test.go`

**Interfaces:**
- Produces: `LoadTokenizer(path string) (*Tokenizer, error)` (path = `tokenizer.json`); `(*Tokenizer).Encode(text string) []int32`.
- Consumes: born's existing BPE machinery if compatible (check `tokenizer/` package first; Qwen2 is GPT-2-style byte-level BPE with a regex pre-tokenizer).

- [ ] **Step 1: Fixture** — harness dumps `tokenizer.cases.json`: 8 strings (ASCII, punctuation, unicode, emoji, the fox prompt, the Qwen-Image template applied to the fox prompt) → token id arrays from `transformers.AutoTokenizer`.
- [ ] **Step 2: Failing test** — table-driven over the cases file, exact `[]int32` equality.
- [ ] **Step 3: Implement** — reuse born BPE if it matches byte-level+regex behavior; otherwise implement (vocab+merges from tokenizer.json, GPT-2 byte mapping, Qwen2 pre-tokenizer regex, special tokens like `<|im_start|>`).
- [ ] **Step 4: Verify pass.**
- [ ] **Step 5: Commit + push** — `"qwenimage: Qwen2 byte-level BPE tokenizer (exact-match parity)"`

### Task 6: Qwen2.5-VL text encoder (embedding extractor)

**Files:**
- Create: `<born>/qwenimage/textencoder.go`, `textencoder_test.go`

**Interfaces:**
- Consumes: Task 4 `LoadF32`; Task 5 `Encode`; born LLaMA building blocks (RMSNorm, RoPE, GQA attention, SwiGLU) — reuse, don't rewrite.
- Produces: `LoadTextEncoder(g *gguf.File) (*TextEncoder, error)`; `(*TextEncoder).Encode(ids []int32) (emb []float32, shape []int, err error)` — returns the hidden states + template-token dropping exactly as diffusers `_get_qwen_prompt_embeds` does (template string, extraction layer, drop count all read from diffusers source and recorded as constants with a comment citing the source line).

- [ ] **Step 1: Tiny-config parity first** — harness builds a random-weight tiny `Qwen2_5_VLTextModel`-equivalent (2 layers, dim 64), dumps weights as fixtures + input ids + output hidden states. Go test builds the same tiny model from those weights, asserts maxAbs ≤ 1e-5. This is the fast red/green loop for attention/RoPE/norm wiring.
- [ ] **Step 2: Implement forward** against the tiny fixture until green. Text path only; vision tensors in the GGUF are deliberately not loaded (strict = all *required* tensors present; maintain an explicit skip-prefix list, e.g. `v.` / `mm.`, asserted non-empty in a test so the intent is visible).
- [ ] **Step 3: Full-weight gate** — harness `dump-text-encoder` runs the real prompt through the real encoder (F32) → fixture. Go test (skips without models): maxAbs ≤ 1e-3, record actual.
- [ ] **Step 4: Template handling** — port the Qwen-Image prompt template + drop logic; fixture covers it (the fixture IS the templated run).
- [ ] **Step 5: Commit + push** — `"qwenimage: Qwen2.5-VL text-path encoder at full-weight parity"`

---

## Phase 3 — MMDiT

### Task 7: DiT structure discovery + one block at tiny-config parity

**Files:**
- Create: `<born>/qwenimage/dit.go`, `dit_test.go`, `<born>/qwenimage/reference/OPS.md`

**Interfaces:**
- Consumes: sd.cpp `qwen_image.hpp` graph construction (blueprint); diffusers `QwenImageTransformer2DModel` (cross-reference); Task 4 loader.
- Produces: `LoadDiT(g *gguf.File) (*DiT, error)`; `(*DiT).Forward(imgTokens, txtEmb []float32, shapes..., timestep float64) (velocity []float32, err error)`; plus exported-for-test block/patchify/rope internals.

- [ ] **Step 1: Discovery artifact** — read sd.cpp qwen_image graph + diffusers class; write `OPS.md`: every op in one transformer block (joint attention with QK RMSNorm, AdaLN-Zero modulation, MLP, RoPE axes/theta, patchify 2×2, final AdaLN+proj), each mapped to an existing born kernel or marked NEW. This is the honest replacement for guessing the kernel list now.
- [ ] **Step 2: NEW kernels TDD** — for each NEW kernel: failing unit test (small hand-computable or torch-fixture case) → implement → pass. One commit per kernel or small group.
- [ ] **Step 3: Tiny-config block parity** — harness builds tiny random-weight `QwenImageTransformer2DModel` (2 layers, small dims), dumps weights + a full forward (inputs: random latent tokens, text emb, timestep) + per-block intermediates. Go: build from same weights, assert block-by-block maxAbs ≤ 1e-4. Shape-diff before value-diff to localize.
- [ ] **Step 4: Commit + push** — `"qwenimage: MMDiT block at tiny-config parity (+ kernel inventory)"`

### Task 8: Full 12B DiT single-step parity

- [ ] **Step 1: Fixture** — harness `dump-dit-step`: real GGUF weights (F32 compute), real text embedding from Task 6's fixture, seeded 512×512 latent noise, timestep from the real schedule → dump input latents + output velocity.
- [ ] **Step 2: Go test** (skips without models): load full DiT via Task 4, run one forward, maxAbs ≤ 1e-3 (record actual + wall time + peak RSS in STATUS.md — this is the first real perf datapoint).
- [ ] **Step 3: Fix divergences** — bisect with per-layer dumps (harness gains `--dump-layer N`) until gate passes.
- [ ] **Step 4: Commit + push** — `"qwenimage: full 12B DiT single-step parity"`

---

## Phase 4 — VAE decoder

### Task 9: VAE decode at parity

**Files:**
- Create: `<born>/qwenimage/vae.go`, `vae_test.go`, `<born>/qwenimage/reference/convert_vae.py`

**Interfaces:**
- Consumes: `convert_vae.py` converts `qwen_image_vae.safetensors` → F16 GGUF via `gguf-py` (so born reuses the Task 4 loader; no safetensors reader in Go, honoring spec scope).
- Produces: `LoadVAE(g *gguf.File) (*VAE, error)`; `(*VAE).Decode(latents []float32, shape []int) (*image.RGBA, error)` including latent scaling/shift constants (from diffusers VAE config, cited in a comment).

- [ ] **Step 1: Convert weights** — write+run `convert_vae.py`; record output file in ORACLE.md.
- [ ] **Step 2: Tiny/structural test** — GroupNorm and any other NEW conv-adjacent kernels get unit tests (torch fixtures) first; Wan-VAE 3D→2D squeeze details resolved against diffusers `AutoencoderKLQwenImage` source.
- [ ] **Step 3: Full parity** — harness `dump-vae`: real latents (from harness e2e, saved at step 8) → decoded RGB f32 fixture. Go: decode same latents, maxAbs ≤ 1e-2 on pixel floats (conv stacks accumulate; also save PNG for eyeball check).
- [ ] **Step 4: Commit + push** — `"qwenimage: VAE decoder at parity"`

---

## Phase 5 — Sampler, pipeline, e2e

### Task 10: Flow-matching Euler scheduler parity

**Files:**
- Create: `<born>/qwenimage/scheduler.go`, `scheduler_test.go`

**Interfaces:**
- Produces: `Sigmas(steps int, imgSeqLen int) []float64` (dynamic shift per model scheduler config, constants cited); `EulerStep(latents, velocity []float32, sigmaCur, sigmaNext float64)`; `NoiseLatents(seed int64, shape []int) []float32` — seeded RNG matching harness dump (noise itself is a fixture: exactness of the *schedule*, not the RNG, is what parity requires — inject harness noise for parity tests, use Go RNG for production).

- [ ] **Step 1: Fixture** — harness `dump-scheduler`: sigmas array for (8 steps, 512×512 seq len) + per-step latent trajectory from a full pipeline run with dumped noise.
- [ ] **Step 2: Failing test** — sigmas exact to 1e-9; trajectory: replay 8 Euler steps in Go using *fixture* velocities, latents match ≤ 1e-5 per step.
- [ ] **Step 3: Implement + pass.**
- [ ] **Step 4: Commit + push** — `"qwenimage: flow-match Euler scheduler at parity"`

### Task 11: `Pipeline.Generate` + `cmd/imagegen` e2e

**Files:**
- Create: `<born>/qwenimage/pipeline.go`, `pipeline_test.go`, `<born>/cmd/imagegen/main.go`

**Interfaces:**
- Produces: `New(modelDir string) (*Pipeline, error)` (expects `krea-dit.gguf`/text-encoder gguf/vae gguf/tokenizer per ORACLE.md layout; missing files → error listing expected layout); `(*Pipeline).Generate(ctx context.Context, prompt string, Options{Width, Height, Steps, Seed}) (image.Image, error)`; ctx checked between DiT steps; progress callback `Options.OnStep func(i, n int, d time.Duration)`.
- CLI: `imagegen -m ~/.cache/manifold/krea2-models -p "..." -o out.png [-w 512 -h 512 --steps 8 --seed 42]`, non-zero exit + layout message on missing files.

- [ ] **Step 1: Deterministic pipeline test** — inject harness noise fixture, run full Go pipeline (skips without models), compare final latents to harness trajectory end ≤ 1e-2, and decoded PNG saved as `go-512-seed42-injected.png` for eyeball.
- [ ] **Step 2: Wire CLI** — flags above; determinism test: same seed twice → identical SHA-256 of PNG bytes.
- [ ] **Step 3: THE RUN** — `imagegen -m ~/.cache/manifold/krea2-models -p "a red fox sitting in fresh snow, golden hour, photorealistic" -o fox-go.png -w 512 -h 512 --steps 8 --seed 42`. Expected: recognizable fox. Record wall time per component + total + peak RSS.
- [ ] **Step 4: Compare** — side-by-side vs `oracle-512-seed42.png` + harness image (visual similarity, not bitwise). Commit `fox-go.png` into `qwenimage/reference/`.
- [ ] **Step 5: Commit + push** — `"qwenimage: end-to-end pure-Go image generation (spike exit criterion)"`

### Task 12: STATUS.md, regressions, wrap-up

- [ ] **Step 1: Regression gate** — `go build ./... && go test ./... 2>&1 | tail -30` on the fork; Supertonic + Moonshine suites green; `go vet ./qwenimage/...` clean; `gofmt -l` empty.
- [ ] **Step 2: Write `qwenimage/STATUS.md`** — per-component parity actuals, perf table (per-step + total + RSS), ops/kernels added, blockers hit + resolutions, go/no-go recommendation for Manifold `/image` productization (integration map is in the spec).
- [ ] **Step 3: Push; update manifold memory + plan checkboxes; report results to user with the image.**

---

## Self-review notes

- Spec coverage: five ports (Tasks 4–10), Phase 0 oracle (Tasks 2–3), e2e CLI (Task 11), STATUS + regression (Task 12), strict loading + parity method + determinism + ctx cancellation all placed. Manifold wiring correctly absent.
- Discovery points are deliberate artifacts (ORACLE.md, OPS.md), not placeholders: the sd.cpp flags, companion file names, and kernel list cannot be truthfully written today, and pretending otherwise would bake in errors.
- Type consistency: `gguf.File.LoadF32` consumed by Tasks 6/7/9; `Encode` naming consistent (tokenizer `Encode(text) []int32`, encoder `Encode(ids) ...` — distinct receivers, acceptable); fixture format identical across all tasks.
