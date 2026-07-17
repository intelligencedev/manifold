// Command tts renders text to a WAV file using the pure-Go Supertonic pipeline.
//
//	go run ./cmd/tts -models <dir> -voice M1 -out out.wav "Hello there."
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/born-ml/born/supertonic"
)

func main() {
	modelDir := flag.String("models", "", "model dir (onnx/ + voice_styles/)")
	voice := flag.String("voice", "M1", "voice id")
	lang := flag.String("lang", "en", "language")
	steps := flag.Int("steps", 8, "flow-matching steps")
	out := flag.String("out", "out.wav", "output wav path")
	flag.Parse()

	text := strings.Join(flag.Args(), " ")
	if *modelDir == "" || text == "" {
		fmt.Fprintln(os.Stderr, "usage: tts -models <dir> [-voice M1] [-out out.wav] <text>")
		os.Exit(2)
	}

	t0 := time.Now()
	tts, err := supertonic.New(*modelDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	loadDur := time.Since(t0)
	t1 := time.Now()
	samples, err := tts.Synthesize(text, *voice, supertonic.Options{Lang: *lang, TotalSteps: *steps})
	if err != nil {
		fmt.Fprintln(os.Stderr, "synthesize:", err)
		os.Exit(1)
	}
	synthDur := time.Since(t1)
	audioSec := float64(len(samples)) / float64(tts.SampleRate())
	fmt.Printf("load %.2fs | synth %.2fs for %.2fs audio | RTF %.2f\n",
		loadDur.Seconds(), synthDur.Seconds(), audioSec, synthDur.Seconds()/audioSec)
	wav := supertonic.WAVBytes(supertonic.FloatToPCM16(samples), tts.SampleRate())
	if err := os.WriteFile(*out, wav, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s: %d samples, %.2fs @ %dHz\n",
		*out, len(samples), float64(len(samples))/float64(tts.SampleRate()), tts.SampleRate())
}
