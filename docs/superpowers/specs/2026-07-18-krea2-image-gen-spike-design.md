# Krea 2 Turbo Image Generation — Pure-Go Feasibility Spike

**Date:** 2026-07-18
**Status:** Approved design, pre-implementation
**Approach:** A+C blend — component-by-component parity ports (the proven Supertonic/Moonshine method), transliterating stable-diffusion.cpp's `qwen_image` implementation as the structural blueprint.

## Goal

Prove that [Krea 2 Turbo](https://huggingface.co/krea/Krea-2-Turbo) — a 12B-parameter Diffusion Transformer in the Qwen-Image (`qwen_image`) MMDiT lineage — can generate an image end-to-end in pure Go via the `github.com/intelligencedev/born` fork, with no CGO and no Python at inference time.

**Exit criterion (success):** `imagegen -p "a red fox in snow" -o out.png` produces a recognizable image, fully in-process Go, on 64 GB Apple Silicon. Any quality; speed is a known cost, not a gate.

**Exit criterion (failure):** a component that cannot reach numeric parity, or a fundamental blocker (an op class born cannot express), documented with evidence in a STATUS.md — same discipline as the TTS spike.

## Model facts (verified 2026-07-18)

- **Krea 2 Turbo:** 12B DiT, distilled turbo — 8 inference steps, CFG/guidance 0.0 (no negative-prompt pass), up to 2048×2048. Official repo ships safetensors only.
- **GGUF weights:** [Abiray/Krea-2-Turbo-GGUF](https://huggingface.co/Abiray/Krea-2-Turbo-GGUF) — Q3_K through Q8_0; architecture tag `qwen_image`; recommends `stable-diffusion.cpp`. Spike baseline: **Q4_K_M (~7.49 GB)**.
- **Lineage:** Qwen-Image MMDiT — components are the DiT itself, a **Qwen2.5-VL text encoder** (text path only; no vision tower needed for t2i), and a **Wan-2.1-style VAE** (16-channel latents). Exact companion GGUF files for sd.cpp to be pinned in Phase 0 (the GGUF repo README's one-liner omits them).
- **Reference implementations:** `stable-diffusion.cpp` (runs this exact GGUF; blueprint + e2e oracle) and HF `diffusers`' Qwen-Image pipeline (readable architecture reference; parity-fixture generator).

## Constraints & context

- **Hardware target:** Apple Silicon, 64 GB unified memory. Memory is NOT the constraint — DiT (Q4 ≈ 7.5 GB) + text encoder + VAE + F32 activations all stay resident. Compute/time is the constraint.
- **CPU-first:** correctness on born's CPU backend first (as TTS did). WebGPU/Metal acceleration is a follow-up, not spike scope.
- **Starting resolution:** 512×512 (4× fewer image tokens than 1024² → directly cuts DiT cost). 1024×1024 once e2e works.
- **born starting point:** fork branch `feat/supertonic-tts` (pinned in manifold go.mod at `f7f8a33`): 57+ ONNX ops incl. the TTS/STT additions, LLaMA GGUF loading with K-quant dequant, transformer primitives (attention, RoPE, GQA, RMSNorm, SwiGLU), Conv1D/Conv2D, BPE/TikToken tokenizers, CPU parallel-matmul optimizations (`bmmParallelF32`, im2col conv).
- **Expected initial perf:** ~12B × 2 × ~1.3k tokens ≈ 30 TFLOPs/DiT-step at 512×512 → minutes per step, plausibly 30–90 min per image unoptimized. Acceptable for the spike; the TTS optimization playbook (parallelism, register tiling, im2col) is the known lever afterward.

## Scope

**In scope**

1. born fork branch `feat/krea2-image` (off `feat/supertonic-tts`, inheriting its ops/fixes) with a new `qwenimage/` package: `New(modelDir)` + `Generate(prompt, Options) → image.Image`, plus `cmd/imagegen` CLI.
2. Phase 0 oracle setup: build stable-diffusion.cpp (Metal) locally, obtain DiT GGUF + companion text-encoder/VAE files, generate a reference image, pin exact file inventory.
3. Five component ports with per-component numeric-parity gates (below), then e2e wiring.
4. STATUS.md with findings, parity numbers, perf measurements, and go/no-go recommendation for productization.

**Out of scope (follow-up projects)**

- Manifold `/image` endpoint, `ImageConfig`, handler/holder wiring (the integration pattern is already mapped: config block → `sync.Once` holder on `app` → dispatcher in `handlers_media.go` → route in `registerAgentRoutes` → persist via `saveGeneratedImages`).
- WebGPU/Metal acceleration; LoRA; img2img/inpainting/editing; robust multi-resolution support; safetensors loading.

## Architecture — five ports in dependency order

Each component is gated on numeric parity before the next begins. Order chosen so every failure is localized.

### 1. Generic GGUF loader

born's GGUF path is LLaMA-shaped (fixed tensor-name expectations). Generalize to: open file → enumerate tensors by name/shape/quant-type → dequantize K-quants (Q4_K/Q5_K/Q6_K/Q8_0 math already exists) to F32 on demand. Mostly API surface, not new math. Strict mode: unknown/missing tensor names fail loudly.

### 2. Qwen2.5-VL text encoder (text path only)

A ~7B Qwen2 decoder stack used as an embedding extractor — run the prompt through, take hidden states; no autoregressive generation, no vision tower. Reuses born's LLaMA components (RMSNorm, RoPE, GQA, SwiGLU); Qwen2 deltas are small (QKV bias, its own BPE vocab via born's tokenizer package). Also implements Qwen-Image's prompt template/embedding extraction convention (which layer's hidden states, any template wrapping — taken from sd.cpp/diffusers).

### 3. Qwen-Image MMDiT (the 12B DiT)

The real port. Dual-stream joint attention over concatenated text+image token sequences, AdaLN-Zero timestep modulation, 2×2 patchify/unpatchify between latent space and token space, qwen_image's RoPE variant for image positions. Transliterated from stable-diffusion.cpp's qwen_image graph construction; cross-read against diffusers' `QwenImageTransformer2DModel` when the C++ is dense. New kernels expected here (exact list discovered during port); each gets a unit test plus its place in the component parity gate.

### 4. VAE decoder

Conv-based upsampler: 16-channel latent → RGB. Wan-2.1-style. born has Conv2D; expect to add GroupNorm and resolve any 3D→2D (video-VAE heritage) squeeze details. Smallest weights; conv-perf lessons from the TTS vocoder apply directly.

### 5. Flow-matching Euler sampler

8 steps, CFG 0.0 (single DiT forward per step), timestep shift schedule per model config, seeded noise for determinism. Pure Go orchestration; trivial compute, fiddly constants — constants taken from sd.cpp/diffusers and locked by the e2e determinism test.

## Parity method & data flow

**Fixture generation (Python reference harness, like TTS `reference/harness.py`):** load the *same dequantized GGUF weights* into diffusers' Qwen-Image modules (via the `gguf` Python package), run each component on fixed inputs, dump input/output tensors to disk. Using identical weights removes quantization as a variable — parity failures then always mean born-side logic bugs.

**Per-component gate:** born test loads the fixture, runs the component, asserts maxAbs difference within tolerance (TTS precedent: 1e-5-ish for F32 pipelines; tolerances recorded per component in STATUS.md). Shape-diff first, then value-diff, to localize the first divergent node.

**E2E oracle:** stable-diffusion.cpp run with the same seed/prompt/steps as final sanity — images should be visually similar (bitwise parity across different samplers/RNG is not expected and not required).

**Inference data flow:**

```
prompt ──BPE──> token ids ──Qwen2.5-VL──> text embeddings
seed ──> latent noise (16ch, H/8 × W/8)
loop 8 steps:
    (text emb, latents, timestep) ──MMDiT──> velocity ──Euler+shift──> latents
latents ──VAE decode──> RGB image ──> PNG
```

## Error handling

- **Loading:** strict everywhere — unknown GGUF tensor names, unmapped ops, or shape mismatches are hard errors, never skipped (the TTS spike's silent-skip trap is the known failure mode this guards against).
- **Runtime:** `Generate` takes a context; cancellation checked between DiT steps (a step is minutes — responsiveness matters). Deterministic given (seed, prompt, options).
- **CLI:** non-zero exit + clear message on missing model files, with the expected `modelDir` layout printed.

## Testing

- Unit tests for every new kernel/op (table-driven, `-race`), as with the TTS ops.
- Per-component parity tests against committed-or-regenerable fixtures (large fixtures regenerated by harness script, not committed).
- Determinism test: fixed seed → identical output hash.
- E2E: CLI run producing a decodable PNG; visual check vs sd.cpp same-seed output.
- Supertonic + Moonshine parity suites must stay green on the fork branch (regression gate — the branch inherits their ops).

## Deliverables

1. Fork branch `feat/krea2-image` pushed to `github.com/intelligencedev/born` with `qwenimage/` + `cmd/imagegen`.
2. Python parity harness + fixture scripts (in-fork, mirroring the TTS layout).
3. STATUS.md: parity numbers per component, perf measurements, ops/kernels added, blockers hit, and go/no-go recommendation for Manifold productization.
4. A generated image.

## Risks

| Risk | Mitigation |
| --- | --- |
| Companion text-encoder/VAE GGUFs unclear from README | Phase 0 pins exact files by making sd.cpp work first; diffusers repo is the fallback weight source |
| MMDiT has op/kernel gaps in born | Expected — same as TTS (8 missing ops then); add with unit tests + parity gate; only an *inexpressible* op class is a blocker |
| CPU perf makes iteration painful | 512×512 + Q4 for the loop; parity fixtures let components be tested without full e2e runs; perf work is explicitly post-spike |
| Qwen2.5-VL embedding-extraction convention subtle (template, layer choice) | Transliterate from sd.cpp exactly; parity fixture on the full encoder output catches it |
| diffusers/gguf Python harness weight-loading mismatch | Harness validates itself first: its e2e output must look right before its fixtures are trusted |
