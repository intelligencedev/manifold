package terminal

import "time"

type outputChunk struct {
	Seq       int64     `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

type outputSnapshot struct {
	Output    string        `json:"output"`
	Chunks    []outputChunk `json:"chunks,omitempty"`
	NextSeq   int64         `json:"next_seq"`
	Truncated bool          `json:"truncated"`
}

type outputBuffer struct {
	maxBytes  int
	chunks    []outputChunk
	bytes     int
	nextSeq   int64
	truncated bool
}

func newOutputBuffer(maxBytes int) *outputBuffer {
	if maxBytes <= 0 {
		maxBytes = defaultTerminalOutputBufferBytes
	}
	return &outputBuffer{maxBytes: maxBytes, chunks: make([]outputChunk, 0, 32), nextSeq: 1}
}

func (b *outputBuffer) append(data []byte, now time.Time) {
	if len(data) == 0 {
		return
	}
	if len(data) > b.maxBytes {
		data = data[len(data)-b.maxBytes:]
		b.truncated = true
	}
	chunk := outputChunk{
		Seq:       b.nextSeq,
		Timestamp: now.UTC(),
		Data:      string(data),
	}
	b.nextSeq++
	b.chunks = append(b.chunks, chunk)
	b.bytes += len(chunk.Data)
	b.trim()
}

func (b *outputBuffer) trim() {
	for b.maxBytes > 0 && b.bytes > b.maxBytes && len(b.chunks) > 0 {
		b.bytes -= len(b.chunks[0].Data)
		b.chunks = b.chunks[1:]
		b.truncated = true
	}
}

func (b *outputBuffer) snapshot(sinceSeq int64, maxBytes int) outputSnapshot {
	if maxBytes <= 0 || maxBytes > b.maxBytes {
		maxBytes = b.maxBytes
	}
	out := outputSnapshot{NextSeq: b.nextSeq}
	if len(b.chunks) == 0 {
		out.Truncated = b.truncated
		return out
	}
	if sinceSeq > 0 && sinceSeq < b.chunks[0].Seq-1 && b.truncated {
		out.Truncated = true
	}

	selected := make([]outputChunk, 0, len(b.chunks))
	total := 0
	for _, chunk := range b.chunks {
		if sinceSeq > 0 && chunk.Seq <= sinceSeq {
			continue
		}
		size := len(chunk.Data)
		if total+size > maxBytes && len(selected) > 0 {
			out.Truncated = true
			break
		}
		if size > maxBytes {
			data := chunk.Data[len(chunk.Data)-maxBytes:]
			selected = append(selected, outputChunk{Seq: chunk.Seq, Timestamp: chunk.Timestamp, Data: data})
			total += len(data)
			out.Truncated = true
			break
		}
		selected = append(selected, chunk)
		total += size
	}
	out.Chunks = selected
	for _, chunk := range selected {
		out.Output += chunk.Data
	}
	if b.truncated && sinceSeq <= 0 {
		out.Truncated = true
	}
	return out
}
