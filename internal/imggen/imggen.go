package imggen

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ComfyProxyRequest represents the expected JSON payload.
type ComfyProxyRequest struct {
	TargetEndpoint string `json:"targetEndpoint"`
	Prompt         string `json:"prompt"`
}

// runFMLXHandler handles FMLX image generation requests.
func RunFMLXHandler(c echo.Context, dataPath string) error {
	var req FMLXRequest
	if err := c.Bind(&req); err != nil {
		return respondWithError(c, http.StatusBadRequest, "Invalid request body")
	}
	if req.Model == "" || req.Prompt == "" || req.Steps == 0 {
		return respondWithError(c, http.StatusBadRequest, "Missing required fields")
	}

	imgPath := fmt.Sprintf("%s/tmp/%s", dataPath, req.Output)
	args := buildFMLXArgs(req, imgPath)

	if err := executeCommand("mflux-generate", args); err != nil {
		return respondWithError(c, http.StatusInternalServerError, fmt.Sprintf("Failed to execute mflux command: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "FMLX command executed successfully",
	})
}

// runSDHandler handles Stable Diffusion image generation requests.
func RunSDHandler(c echo.Context) error {
	var req SDRequest
	if err := c.Bind(&req); err != nil {
		return respondWithError(c, http.StatusBadRequest, "Invalid request body")
	}
	if !validateSDRequest(req) {
		return respondWithError(c, http.StatusBadRequest, "Missing required fields")
	}

	args := buildSDArgs(req)
	if err := executeCommand("./sd", args); err != nil {
		return respondWithError(c, http.StatusInternalServerError, fmt.Sprintf("Failed to execute sd command: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Stable Diffusion command executed successfully",
	})
}

// comfyProxyHandler handles requests to the ComfyUI proxy.
func ComfyProxyHandler(c echo.Context) error {
	var reqBody ComfyProxyRequest
	if err := c.Bind(&reqBody); err != nil {
		return respondWithError(c, http.StatusBadRequest, "Invalid request body")
	}
	if reqBody.TargetEndpoint == "" {
		return respondWithError(c, http.StatusBadRequest, "TargetEndpoint is required")
	}

	templateData, err := parseComfyTemplate()
	if err != nil {
		return respondWithError(c, http.StatusInternalServerError, "Failed to unmarshal comfy template")
	}

	uuid := generateUUID()
	if err := updateComfyTemplate(templateData, reqBody.Prompt, uuid); err != nil {
		return respondWithError(c, http.StatusInternalServerError, err.Error())
	}

	jsonData, err := json.Marshal(map[string]interface{}{"prompt": templateData})
	if err != nil {
		return respondWithError(c, http.StatusInternalServerError, "Failed to marshal updated template")
	}

	if err := sendComfyRequest(reqBody.TargetEndpoint, jsonData, uuid, c); err != nil {
		return err
	}

	return nil
}

// Helper functions

func respondWithError(c echo.Context, status int, message string) error {
	return c.JSON(status, map[string]string{"error": message})
}

func buildFMLXArgs(req FMLXRequest, imgPath string) []string {
	return []string{
		"--model", req.Model,
		"--prompt", req.Prompt,
		"--steps", fmt.Sprintf("%d", req.Steps),
		"--seed", fmt.Sprintf("%d", req.Seed),
		"-q", fmt.Sprintf("%d", req.Quality),
		"--output", imgPath,
	}
}

func buildSDArgs(req SDRequest) []string {
	args := []string{
		"--diffusion-model", req.DiffusionModel,
		"--type", req.Type,
		"--clip_l", req.ClipL,
		"--t5xxl", req.T5xxl,
		"--vae", req.VAE,
		"--cfg-scale", fmt.Sprintf("%.1f", req.CfgScale),
		"--steps", fmt.Sprintf("%d", req.Steps),
		"--sampling-method", req.SamplingMethod,
		"-H", fmt.Sprintf("%d", req.Height),
		"-W", fmt.Sprintf("%d", req.Width),
		"--seed", fmt.Sprintf("%d", req.Seed),
		"-p", req.Prompt,
		"--output", req.Output,
	}
	if req.Threads > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", req.Threads))
	}
	if req.NegativePrompt != "" {
		args = append(args, "-n", req.NegativePrompt)
	}
	if req.StyleRatio > 0 {
		args = append(args, "--style-ratio", fmt.Sprintf("%.1f", req.StyleRatio))
	}
	if req.ControlStrength > 0 {
		args = append(args, "--control-strength", fmt.Sprintf("%.1f", req.ControlStrength))
	}
	if req.ClipSkip > 0 {
		args = append(args, "--clip-skip", fmt.Sprintf("%d", req.ClipSkip))
	}
	if req.SLGScale > 0 {
		args = append(args, "--slg-scale", fmt.Sprintf("%.1f", req.SLGScale))
	}
	for _, v := range req.SkipLayers {
		args = append(args, "--skip-layers", fmt.Sprintf("%d", v))
	}
	if req.SkipLayerStart > 0 {
		args = append(args, "--skip-layer-start", fmt.Sprintf("%.3f", req.SkipLayerStart))
	}
	if req.SkipLayerEnd > 0 {
		args = append(args, "--skip-layer-end", fmt.Sprintf("%.3f", req.SkipLayerEnd))
	}
	return args
}

func executeCommand(command string, args []string) error {
	cmd := exec.Command(command, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go logOutput(stdout, "stdout", &wg)
	go logOutput(stderr, "stderr", &wg)

	if err := cmd.Wait(); err != nil {
		wg.Wait()
		return fmt.Errorf("command execution failed: %w", err)
	}
	wg.Wait()
	return nil
}

func logOutput(pipe io.ReadCloser, label string, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		log.Printf("[%s] %s", label, scanner.Text())
	}
}

func parseComfyTemplate() (map[string]interface{}, error) {
	var templateData map[string]interface{}
	if err := json.Unmarshal([]byte(comfyTemplate), &templateData); err != nil {
		return nil, err
	}
	return templateData, nil
}

func updateComfyTemplate(templateData map[string]interface{}, prompt, uuid string) error {
	if node9, ok := templateData["9"].(map[string]interface{}); ok {
		if inputs, ok := node9["inputs"].(map[string]interface{}); ok {
			inputs["filename_prefix"] = uuid
		} else {
			return fmt.Errorf("invalid template structure in node 9")
		}
	} else {
		return fmt.Errorf("node 9 not found in template")
	}

	if node6, ok := templateData["6"].(map[string]interface{}); ok {
		if inputs, ok := node6["inputs"].(map[string]interface{}); ok {
			inputs["text"] = prompt
		} else {
			return fmt.Errorf("invalid template structure in node 6")
		}
	} else {
		return fmt.Errorf("node 6 not found in template")
	}

	return nil
}

func sendComfyRequest(targetEndpoint string, jsonData []byte, uuid string, c echo.Context) error {
	proxyReq, err := http.NewRequest("POST", targetEndpoint, bytes.NewReader(jsonData))
	if err != nil {
		return respondWithError(c, http.StatusInternalServerError, "Failed to create request to target endpoint")
	}
	proxyReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		return respondWithError(c, http.StatusInternalServerError, fmt.Sprintf("Error making request to target endpoint: %v", err))
	}
	defer resp.Body.Close()

	return waitForImage(targetEndpoint, uuid, c)
}

func waitForImage(baseURL, uuid string, c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Replace the "/prompt" part of the URL with "/view"
	baseURL = baseURL[:len(baseURL)-len("/prompt")]
	// Construct the image URL
	imageURL := fmt.Sprintf("%s/view?filename=%s_00001_.png&subfolder=&type=output", baseURL, uuid)
	log.Printf("Waiting for image at %s", imageURL)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return respondWithError(c, http.StatusGatewayTimeout, "Timeout waiting for image generation")
		case <-ticker.C:
			if err := checkImageAvailability(imageURL, c); err == nil {
				return nil
			}
		}
	}
}

func checkImageAvailability(imageURL string, c echo.Context) error {
	imgResp, err := http.Get(imageURL)
	if err != nil {
		return err
	}
	defer imgResp.Body.Close()

	if imgResp.StatusCode == http.StatusOK {
		c.Response().Header().Set("Content-Type", imgResp.Header.Get("Content-Type"))
		return c.Stream(http.StatusOK, imgResp.Header.Get("Content-Type"), imgResp.Body)
	}
	return fmt.Errorf("image not available")
}

func generateUUID() string {
	return uuid.New().String()
}

func validateSDRequest(req SDRequest) bool {
	return req.DiffusionModel != "" && req.Type != "" && req.ClipL != "" && req.T5xxl != "" && req.VAE != "" && req.Prompt != "" && req.Output != ""
}

var comfyTemplate = `{
  "6": {
    "inputs": {
      "text": "a luxury breakfast photograph",
      "speak_and_recognation": true,
      "clip": [
        "11",
        0
      ]
    },
    "class_type": "CLIPTextEncode",
    "_meta": {
      "title": "CLIP Text Encode (Positive Prompt)"
    }
  },
  "8": {
    "inputs": {
      "samples": [
        "13",
        0
      ],
      "vae": [
        "10",
        0
      ]
    },
    "class_type": "VAEDecode",
    "_meta": {
      "title": "VAE Decode"
    }
  },
  "9": {
    "inputs": {
      "filename_prefix": "manifold",
      "images": [
        "8",
        0
      ]
    },
    "class_type": "SaveImage",
    "_meta": {
      "title": "Save Image"
    }
  },
  "10": {
    "inputs": {
      "vae_name": "ae.safetensors"
    },
    "class_type": "VAELoader",
    "_meta": {
      "title": "Load VAE"
    }
  },
  "11": {
    "inputs": {
      "clip_name1": "t5xxl_fp16.safetensors",
      "clip_name2": "clip_l.safetensors",
      "type": "flux"
    },
    "class_type": "DualCLIPLoader",
    "_meta": {
      "title": "DualCLIPLoader"
    }
  },
  "13": {
    "inputs": {
      "noise": [
        "25",
        0
      ],
      "guider": [
        "22",
        0
      ],
      "sampler": [
        "16",
        0
      ],
      "sigmas": [
        "17",
        0
      ],
      "latent_image": [
        "27",
        0
      ]
    },
    "class_type": "SamplerCustomAdvanced",
    "_meta": {
      "title": "SamplerCustomAdvanced"
    }
  },
  "16": {
    "inputs": {
      "sampler_name": "euler"
    },
    "class_type": "KSamplerSelect",
    "_meta": {
      "title": "KSamplerSelect"
    }
  },
  "17": {
    "inputs": {
      "scheduler": "simple",
      "steps": 20,
      "denoise": 1,
      "model": [
        "30",
        0
      ]
    },
    "class_type": "BasicScheduler",
    "_meta": {
      "title": "BasicScheduler"
    }
  },
  "22": {
    "inputs": {
      "model": [
        "30",
        0
      ],
      "conditioning": [
        "26",
        0
      ]
    },
    "class_type": "BasicGuider",
    "_meta": {
      "title": "BasicGuider"
    }
  },
  "25": {
    "inputs": {
      "noise_seed": 284515125733667
    },
    "class_type": "RandomNoise",
    "_meta": {
      "title": "RandomNoise"
    }
  },
  "26": {
    "inputs": {
      "guidance": 3.5,
      "conditioning": [
        "6",
        0
      ]
    },
    "class_type": "FluxGuidance",
    "_meta": {
      "title": "FluxGuidance"
    }
  },
  "27": {
    "inputs": {
      "width": 1280,
      "height": 768,
      "batch_size": 1
    },
    "class_type": "EmptySD3LatentImage",
    "_meta": {
      "title": "EmptySD3LatentImage"
    }
  },
  "30": {
    "inputs": {
      "max_shift": 1.15,
      "base_shift": 0.5,
      "width": 1024,
      "height": 1024,
      "model": [
        "38",
        0
      ]
    },
    "class_type": "ModelSamplingFlux",
    "_meta": {
      "title": "ModelSamplingFlux"
    }
  },
  "38": {
    "inputs": {
      "unet_name": "flux1-dev-Q8_0.gguf"
    },
    "class_type": "UnetLoaderGGUF",
    "_meta": {
      "title": "Unet Loader (GGUF)"
    }
  }
}`
