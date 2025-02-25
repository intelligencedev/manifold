package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ComfyProxyRequest represents the expected JSON payload.
type ComfyProxyRequest struct {
	TargetEndpoint string `json:"targetEndpoint"`
	Prompt         string `json:"prompt"`
}

var comfyTemplate = `{
  "6": {
    "inputs": {
      "text": "cute anime girl with massive fluffy fennec ears and a big fluffy tail blonde messy long hair blue eyes wearing a maid outfit with a long black gold leaf pattern dress and a white apron mouth open holding a fancy black forest cake with candles on top in the kitchen of an old dark Victorian mansion lit by candlelight with a bright window to the foggy forest and very expensive stuff everywhere",
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
      "width": 1024,
      "height": 1024,
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

func comfyProxyHandler(c echo.Context) error {
	var reqBody ComfyProxyRequest
	if err := c.Bind(&reqBody); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if reqBody.TargetEndpoint == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "TargetEndpoint is required"})
	}

	// Unmarshal the hardcoded template.
	var templateData map[string]interface{}
	if err := json.Unmarshal([]byte(comfyTemplate), &templateData); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to unmarshal comfy template"})
	}

	// seedNodes := []string{"109", "157", "248", "260"}

	// // Generate a random number between 1 and 18446744073709519872
	// seed := math.MaxUint64 * rand.Float64()

	// log.Default().Printf("Using seed: %v", seed)

	// for _, node := range seedNodes {
	// 	if nodeData, ok := templateData[node].(map[string]interface{}); ok {
	// 		if inputs, ok := nodeData["inputs"].(map[string]interface{}); ok {
	// 			inputs["noise_seed"] = seed
	// 		} else {
	// 			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Invalid template structure in node " + node})
	// 		}
	// 	} else {
	// 		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Node " + node + " not found in template"})
	// 	}
	// }

	// Generate UUID for the image filename
	uuid := generateUUID()
	if node9, ok := templateData["9"].(map[string]interface{}); ok {
		if inputs, ok := node9["inputs"].(map[string]interface{}); ok {
			inputs["filename_prefix"] = uuid
		} else {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Invalid template structure in node 9"})
		}
	} else {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Node 9 not found in template"})
	}

	// Update prompt nodes
	if node6, ok := templateData["6"].(map[string]interface{}); ok {
		if inputs, ok := node6["inputs"].(map[string]interface{}); ok {
			inputs["text"] = reqBody.Prompt
		} else {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Invalid template structure in node 6"})
		}
	} else {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Node 6 not found in template"})
	}

	// if node106, ok := templateData["106"].(map[string]interface{}); ok {
	// 	if inputs, ok := node106["inputs"].(map[string]interface{}); ok {
	// 		inputs["clip_l"] = reqBody.Prompt
	// 		inputs["t5xxl"] = reqBody.Prompt
	// 	} else {
	// 		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Invalid template structure in node 106"})
	// 	}
	// }

	// if node189, ok := templateData["189"].(map[string]interface{}); ok {
	// 	if inputs, ok := node189["inputs"].(map[string]interface{}); ok {
	// 		inputs["text"] = reqBody.Prompt
	// 	} else {
	// 		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Invalid template structure in node 189"})
	// 	}
	// }

	payload := map[string]interface{}{
		"prompt": templateData,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to marshal updated template"})
	}

	// Send request to ComfyUI
	proxyReq, err := http.NewRequest("POST", reqBody.TargetEndpoint, bytes.NewReader(jsonData))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create request to target endpoint"})
	}
	proxyReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Error making request to target endpoint: %v", err)})
	}
	defer resp.Body.Close()

	// Set up timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	baseURL := reqBody.TargetEndpoint[:strings.LastIndex(reqBody.TargetEndpoint, "/")]
	imageURL := fmt.Sprintf("%s/view?filename=%s_00001_.png&subfolder=&type=output", baseURL, uuid)

	log.Default().Printf("Waiting for image at %s", imageURL)

	// Keep checking for the image until timeout
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return c.JSON(http.StatusGatewayTimeout, map[string]string{"error": "Timeout waiting for image generation"})
		case <-ticker.C:
			imgResp, err := http.Get(imageURL)
			if err != nil {
				continue
			}

			if imgResp.StatusCode == http.StatusOK {
				defer imgResp.Body.Close()
				c.Response().Header().Set("Content-Type", imgResp.Header.Get("Content-Type"))

				return c.Stream(http.StatusOK, imgResp.Header.Get("Content-Type"), imgResp.Body)
			}
			imgResp.Body.Close()
		}
	}
}

func generateUUID() string {
	return uuid.New().String()
}
