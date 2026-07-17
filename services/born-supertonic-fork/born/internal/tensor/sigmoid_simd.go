package tensor

// sigmoidF32 is the optional vendored-SIMD sigmoid: out[i] = 1/(1+exp(-in[i])).
//
// It is nil by default and wired in by an arch-specific init when the CPU
// supports the required instructions (AVX2+FMA on amd64, see
// sigmoid_simd_amd64.go). When non-nil, Sigmoid (and the SiLU fast path) use it
// instead of the per-element scalar loop; when nil they use the scalar loop
// unchanged. out and in must have the same length.
var sigmoidF32 func(out, in []float32)
