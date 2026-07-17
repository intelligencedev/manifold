package operators

// powConstF32 is the optional vendored-SIMD pow with a constant exponent:
// out[i] = pow(in[i], c), computed as exp(c*log(in[i])). It targets the ONNX Pow
// op's scalar-exponent case on non-negative inputs (e.g. mel-spectrogram power
// compression).
//
// It is nil by default and wired by an arch-specific init when the CPU supports
// AVX2+FMA (see pow_simd_amd64.go). When non-nil, handlePow uses it for the
// scalar-exponent case; otherwise the scalar math.Pow loop runs. out and in must
// have the same length.
//
// Domain: inputs must be non-negative. The kernel flushes x<=0 to 0, which
// matches math.Pow for x==0 with c>0 (the BirdNET mel-spectrogram use). It does
// NOT reproduce math.Pow on a negative base (math.Pow yields NaN for a
// non-integer exponent and the signed root for an integer one; the kernel yields
// 0), so a model that feeds a negative base into a scalar-exponent Pow would
// diverge from the scalar path on AVX2 CPUs. No born model does this.
var powConstF32 func(out, in []float32, c float32)
