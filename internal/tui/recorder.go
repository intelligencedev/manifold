package tui

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gordonklaus/portaudio"
)

// Recorder captures audio from the default input device and writes a WAV file
// when stopped. It uses PortAudio for cross-platform capture and writes 16-bit
// PCM mono samples at the configured sample rate.
type Recorder struct {
	sync.Mutex
	stream          *portaudio.Stream
	buf             []int16
	sampleRate      float64
	framesPerBuffer int
	running         bool
	outfile         *os.File
	samples         []int16
	floatSamples    []float32 // For Whisper processing
}

// NewRecorder creates a recorder configured to capture at the given sampleRate.
func NewRecorder(sampleRate float64) (*Recorder, error) {
	if err := portaudio.Initialize(); err != nil {
		return nil, fmt.Errorf("portaudio initialize: %w", err)
	}

	return &Recorder{
		sampleRate:      sampleRate,
		framesPerBuffer: 1024,
	}, nil
} // Start begins recording to a UUID-named .wav file in the current working directory.
func (r *Recorder) Start() error {
	r.Lock()
	defer r.Unlock()
	if r.running {
		return fmt.Errorf("already recording")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	fname := uuid.New().String() + ".wav"
	path := filepath.Join(cwd, fname)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	// Write placeholder WAV header - we'll fix it when we stop
	if err := r.writeWAVHeader(f, 0); err != nil {
		_ = f.Close()
		return fmt.Errorf("write header: %w", err)
	}

	// open portaudio stream for int16 samples
	in := make([]int16, r.framesPerBuffer)
	stream, err := portaudio.OpenDefaultStream(1, 0, r.sampleRate, len(in), &in)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("open stream: %w", err)
	}

	if err := stream.Start(); err != nil {
		stream.Close()
		_ = f.Close()
		return fmt.Errorf("start stream: %w", err)
	}

	// store state
	r.stream = stream
	r.buf = in
	r.running = true
	r.outfile = f
	r.samples = nil      // reset samples buffer
	r.floatSamples = nil // reset float samples buffer

	// spawn a goroutine to read samples until stopped
	go func() {
		for {
			// read blocks
			err := stream.Read()
			if err != nil {
				break
			}

			// append samples to our buffer
			r.Lock()
			r.samples = append(r.samples, r.buf...)
			// Convert int16 to float32 for Whisper
			for _, sample := range r.buf {
				r.floatSamples = append(r.floatSamples, float32(sample)/32768.0)
			}
			r.Unlock()

			// check running flag periodically
			r.Lock()
			running := r.running
			r.Unlock()
			if !running {
				break
			}
		}

		// cleanup
		r.Lock()
		if r.stream != nil {
			_ = r.stream.Stop()
			_ = r.stream.Close()
			r.stream = nil
		}
		r.running = false
		r.Unlock()
	}()

	return nil
}

// Stop ends recording and returns the saved filename. Note: the implementation
// currently writes directly to the file and closes it; we return the filename
// that was created. If called when not recording, error is returned.
func (r *Recorder) Stop() (string, error) {
	r.Lock()
	if !r.running {
		r.Unlock()
		return "", fmt.Errorf("not recording")
	}
	// capture path before we mutate
	var fname string
	if r.outfile != nil {
		fname = r.outfile.Name()
	}
	// flip running flag so goroutine exits
	r.running = false
	// Release lock while we wait for goroutine to close resources
	r.Unlock()

	// wait briefly for goroutine to finish
	t := time.Now()
	for {
		r.Lock()
		running := r.running
		r.Unlock()
		if !running {
			// Goroutine has stopped, give it a moment to finish cleanup
			time.Sleep(100 * time.Millisecond)
			break
		}
		if time.Since(t) > time.Second*5 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	r.Lock()
	defer r.Unlock()

	// Write all samples to file and fix header
	if r.outfile != nil {
		// Write sample data
		for _, sample := range r.samples {
			binary.Write(r.outfile, binary.LittleEndian, sample)
		}

		// Fix WAV header with correct sizes
		numSamples := uint32(len(r.samples))
		dataSize := numSamples * 2 // 16-bit samples = 2 bytes each
		riffSize := 36 + dataSize  // header size + data size

		// Update RIFF chunk size at offset 4
		r.outfile.Seek(4, 0)
		binary.Write(r.outfile, binary.LittleEndian, riffSize)

		// Update data chunk size at offset 40
		r.outfile.Seek(40, 0)
		binary.Write(r.outfile, binary.LittleEndian, dataSize)

		r.outfile.Close()
		r.outfile = nil
	}

	// Don't terminate portaudio here - keep it initialized for future recordings
	if fname == "" {
		return "", fmt.Errorf("unknown output file")
	}
	return fname, nil
}

// GetFloatSamples returns the recorded samples as float32 for Whisper processing
func (r *Recorder) GetFloatSamples() []float32 {
	r.Lock()
	defer r.Unlock()
	// Return a copy to avoid race conditions
	samples := make([]float32, len(r.floatSamples))
	copy(samples, r.floatSamples)
	return samples
}

// Close terminates PortAudio and cleans up resources
func (r *Recorder) Close() error {
	r.Lock()
	defer r.Unlock()

	if r.running {
		// Stop any active recording first
		r.running = false
		if r.stream != nil {
			r.stream.Stop()
			r.stream.Close()
			r.stream = nil
		}
		if r.outfile != nil {
			r.outfile.Close()
			r.outfile = nil
		}
	}

	return portaudio.Terminate()
}

// writeWAVHeader writes a standard WAV file header
func (r *Recorder) writeWAVHeader(f *os.File, dataSize uint32) error {
	sampleRate := uint32(r.sampleRate)
	bitsPerSample := uint16(16)
	numChannels := uint16(1)
	byteRate := sampleRate * uint32(numChannels) * uint32(bitsPerSample) / 8
	blockAlign := numChannels * bitsPerSample / 8

	// RIFF header
	f.Write([]byte("RIFF"))
	binary.Write(f, binary.LittleEndian, uint32(36+dataSize)) // chunk size
	f.Write([]byte("WAVE"))

	// fmt chunk
	f.Write([]byte("fmt "))
	binary.Write(f, binary.LittleEndian, uint32(16)) // fmt chunk size
	binary.Write(f, binary.LittleEndian, uint16(1))  // PCM format
	binary.Write(f, binary.LittleEndian, numChannels)
	binary.Write(f, binary.LittleEndian, sampleRate)
	binary.Write(f, binary.LittleEndian, byteRate)
	binary.Write(f, binary.LittleEndian, blockAlign)
	binary.Write(f, binary.LittleEndian, bitsPerSample)

	// data chunk header
	f.Write([]byte("data"))
	binary.Write(f, binary.LittleEndian, dataSize)

	return nil
}
