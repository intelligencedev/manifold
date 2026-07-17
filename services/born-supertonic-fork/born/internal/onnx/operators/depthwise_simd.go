package operators

// depthwise3x3F32 is the optional vendored-SIMD 3x3 depthwise convolution for the
// stride=1, padding=0 case: each (n,c) plane is convolved with channel c's nine
// taps (weight[(plane%c)*9 : +9]) into out.
//
// It is nil by default and wired in by an arch-specific init when the CPU supports
// the required instructions (AVX2+FMA on amd64, see depthwise_simd_amd64.go). When
// non-nil, depthwiseConvForward3x3Float32 uses it for stride=1; stride>1 always
// uses the scalar path. The input is already padded by the caller (padding 0).
//
// Tests swap this var (saving and restoring it) to force the scalar or SIMD path,
// so the depthwise tests must not run in parallel.
var depthwise3x3F32 func(out, in, weight []float32, n, c, hp, wp, hOut, wOut int)
