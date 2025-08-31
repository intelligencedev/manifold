# whisper.cpp speech to text

This guide covers MacOS only. For other operating systems refer to the [whisper.cpp](https://github.com/ggml-org/whisper.cpp?tab=readme-ov-file#quick-start) documentation.

## Environment Setup

```
# Clone Project
git clone https://github.com/ggml-org/whisper.cpp.git
cd whisper.cpp

# Setup conda and virtualenv
conda create -n py311-whisper python=3.11 -y
conda activate py311-whisper
python -m venv .venv
source .venv/bin/activate

# Install required dependencies
pip install torch torchvision
pip install ane_transformers             
pip install openai-whisper
pip install coremltools

# Convert GGML model to CoreML
./models/generate-coreml-model.sh base.en

# Build whisper.cpp with CoreML support
cmake -B build -DWHISPER_COREML=1        
cmake --build build -j --config Release
```

## Example Commands

IMPORTANT: whisper-cli currently runs only with 16-bit WAV files.

CoreML: The first run on a device is slow, since the ANE service compiles the Core ML model to some device-specific format. Next runs are faster.

```
./build/bin/whisper-cli -f 16bit.wav

# Example output

main: processing '16bit.wav' (524288 samples, 32.8 sec), 4 threads, 1 processors, 5 beams + best of 5, lang = en, task = transcribe, timestamps = 1 ...


[00:00:00.000 --> 00:00:08.680]   Between code and quiet, circuit stream, a lattice hum were thought and light convene.
[00:00:08.680 --> 00:00:16.160]   It learns the hush of human doubt and gleam, maps scattered stories into patterns seen.
[00:00:16.160 --> 00:00:19.960]   Not flesh but logic, not soul but song.
[00:00:19.960 --> 00:00:23.560]   It query stars and asks where we belong.
[00:00:23.560 --> 00:00:30.440]   We teach, it echoes, new mirrors made, together building dawns from data's braid.
```

# Go Bindings

The official [go bindings](https://github.com/ggml-org/whisper.cpp/tree/master/bindings/go) can be used to integrate whisper.cpp into your project.

```
cd whisper.cpp/bindings/go

# This will compile a static libwhisper.a in a build folder, download a model file, then run the tests.
make test

# To build the examples:
make examples

# Download all the models
./build_go/go-model-download -out models

# Test the models (CoreML in this case)
# Note we do not pass the -f named param for Go
./build/go-whisper -model models/ggml-small.en.bin 16bit.wav

# How to run in this project (models must be staged manually)
./run-whisper-go.sh -model ./models/ggml-small.en.bin ./test.wav
```