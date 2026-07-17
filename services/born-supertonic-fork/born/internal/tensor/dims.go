package tensor

// ConvDims holds pre-computed dimensions for 2D convolution operations.
type ConvDims struct {
	N, CIn, H, W    int // Input dimensions
	COut, KH, KW    int // Kernel dimensions
	HOut, WOut      int // Output dimensions
	Stride, Padding int // Convolution parameters
}

// PoolDims holds pre-computed dimensions for 2D pooling operations.
type PoolDims struct {
	N, C, H, W      int // Input dimensions
	KH, KW          int // Kernel dimensions
	HOut, WOut      int // Output dimensions
	Stride, Padding int // Pooling parameters
}
