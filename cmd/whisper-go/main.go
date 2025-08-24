package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"unsafe"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

func main() {
	var modelPath string
	var audioPath string

	// Parse command line arguments
	flag.StringVar(&modelPath, "model", "", "Path to the whisper model file")
	flag.Parse()

	// Get audio file path from remaining arguments
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s -model <model_path> <audio_file>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s -model models/ggml-small.en.bin audio.wav\n", os.Args[0])
		os.Exit(1)
	}
	audioPath = args[0]

	if modelPath == "" {
		fmt.Fprintf(os.Stderr, "Error: -model flag is required\n")
		os.Exit(1)
	}

	// Check if files exist
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		log.Fatalf("Model file does not exist: %s", modelPath)
	}
	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		log.Fatalf("Audio file does not exist: %s", audioPath)
	}

	fmt.Printf("Loading model: %s\n", modelPath)
	fmt.Printf("Processing audio: %s\n", audioPath)

	// Load the model
	model, err := whisper.New(modelPath)
	if err != nil {
		log.Fatalf("Failed to load model: %v", err)
	}
	defer model.Close()

	// Load audio data
	samples, err := loadWAVFile(audioPath)
	if err != nil {
		log.Fatalf("Failed to load audio file: %v", err)
	}

	fmt.Printf("Loaded %d audio samples\n", len(samples))

	// Process audio
	context, err := model.NewContext()
	if err != nil {
		log.Fatalf("Failed to create context: %v", err)
	}

	fmt.Println("Processing audio...")
	if err := context.Process(samples, nil, nil, nil); err != nil {
		log.Fatalf("Failed to process audio: %v", err)
	}

	// Print out the results
	fmt.Println("Transcription results:")
	fmt.Println(string(make([]byte, 60)))
	for {
		segment, err := context.NextSegment()
		if err != nil {
			break
		}
		fmt.Printf("[%6s->%6s] %s\n", segment.Start, segment.End, segment.Text)
	}
}

// WAVHeader represents the header of a WAV file
type WAVHeader struct {
	ChunkID       [4]byte
	ChunkSize     uint32
	Format        [4]byte
	Subchunk1ID   [4]byte
	Subchunk1Size uint32
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
	Subchunk2ID   [4]byte
	Subchunk2Size uint32
}

// loadWAVFile loads a WAV file and converts it to float32 samples for whisper
func loadWAVFile(filepath string) ([]float32, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	var header WAVHeader
	err = binary.Read(file, binary.LittleEndian, &header)
	if err != nil {
		return nil, fmt.Errorf("failed to read WAV header: %v", err)
	}

	// Check if it's a valid WAV file
	if string(header.ChunkID[:]) != "RIFF" || string(header.Format[:]) != "WAVE" {
		return nil, fmt.Errorf("not a valid WAV file")
	}

	fmt.Printf("WAV Info: %d channels, %d Hz, %d bits per sample\n",
		header.NumChannels, header.SampleRate, header.BitsPerSample)

	// Read audio data
	audioData := make([]byte, header.Subchunk2Size)
	_, err = io.ReadFull(file, audioData)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio data: %v", err)
	}

	// Convert to float32 samples
	var samples []float32

	switch header.BitsPerSample {
	case 16:
		// Convert 16-bit signed PCM to float32
		for i := 0; i < len(audioData); i += 2 {
			if i+1 < len(audioData) {
				sample := int16(binary.LittleEndian.Uint16(audioData[i : i+2]))
				// Convert to float32 in range [-1.0, 1.0]
				floatSample := float32(sample) / 32768.0
				samples = append(samples, floatSample)
			}
		}
	case 32:
		// Assume 32-bit float
		for i := 0; i < len(audioData); i += 4 {
			if i+3 < len(audioData) {
				bits := binary.LittleEndian.Uint32(audioData[i : i+4])
				sample := *(*float32)(unsafe.Pointer(&bits))
				samples = append(samples, sample)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported bits per sample: %d", header.BitsPerSample)
	}

	// If stereo, convert to mono by averaging channels
	if header.NumChannels == 2 {
		monoSamples := make([]float32, len(samples)/2)
		for i := 0; i < len(monoSamples); i++ {
			monoSamples[i] = (samples[i*2] + samples[i*2+1]) / 2.0
		}
		samples = monoSamples
	}

	// Whisper expects 16kHz audio, so we might need to resample
	// For now, just print a warning if it's not 16kHz
	if header.SampleRate != 16000 {
		fmt.Printf("Warning: Audio is %d Hz, whisper expects 16000 Hz. Results may be suboptimal.\n", header.SampleRate)
	}

	return samples, nil
}
