package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

// =====================
// Existing Argument Types
// =====================

// HelloArgs represents the arguments for the hello tool
type HelloArgs struct {
	Name string `json:"name" jsonschema:"required,description=The name to say hello to"`
}

// CalculateArgs represents the arguments for the calculate tool
type CalculateArgs struct {
	Operation string  `json:"operation" jsonschema:"required,enum=add,enum=subtract,enum=multiply,enum=divide,description=The mathematical operation to perform"`
	A         float64 `json:"a" jsonschema:"required,description=First number"`
	B         float64 `json:"b" jsonschema:"required,description=Second number"`
}

// TimeArgs represents the arguments for the current time tool
type TimeArgs struct {
	Format string `json:"format,omitempty" jsonschema:"description=Optional time format (default: RFC3339)"`
}

// PromptArgs represents the arguments for custom prompts
type PromptArgs struct {
	Input string `json:"input" jsonschema:"required,description=The input text to process"`
}

// WeatherArgs represents the arguments for the weather tool
type WeatherArgs struct {
	Longitude float64 `json:"longitude" jsonschema:"required,description=The longitude of the location to get the weather for"`
	Latitude  float64 `json:"latitude" jsonschema:"required,description=The latitude of the location to get the weather for"`
}

// =====================
// New File System Tools (Argument Types)
// =====================

// ReadFileArgs is used by the "read_file" tool.
type ReadFileArgs struct {
	Path string `json:"path" jsonschema:"required,description=Path to the file to read"`
}

// WriteFileArgs is used by the "write_file" tool.
type WriteFileArgs struct {
	Path    string `json:"path" jsonschema:"required,description=Path to the file to write"`
	Content string `json:"content" jsonschema:"required,description=Content to write into the file"`
}

// ListDirectoryArgs is used by the "list_directory" tool.
type ListDirectoryArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory path to list"`
}

// CreateDirectoryArgs is used by the "create_directory" tool.
type CreateDirectoryArgs struct {
	Path string `json:"path" jsonschema:"required,description=Directory path to create"`
}

// MoveFileArgs is used by the "move_file" tool.
type MoveFileArgs struct {
	Source      string `json:"source" jsonschema:"required,description=Source file/directory path"`
	Destination string `json:"destination" jsonschema:"required,description=Destination file/directory path"`
}

func main() {
	// Create a transport for the server
	serverTransport := stdio.NewStdioServerTransport()

	// Create a new server with the transport
	server := mcp.NewServer(serverTransport)

	// --------------------------
	// Existing Tools
	// --------------------------
	err := server.RegisterTool("hello", "Says hello to the provided name", func(args HelloArgs) (*mcp.ToolResponse, error) {
		message := fmt.Sprintf("Hello, %s!", args.Name)
		return mcp.NewToolResponse(mcp.NewTextContent(message)), nil
	})
	if err != nil {
		panic(err)
	}

	err = server.RegisterTool("calculate", "Performs basic mathematical operations", func(args CalculateArgs) (*mcp.ToolResponse, error) {
		var result float64
		switch args.Operation {
		case "add":
			result = args.A + args.B
		case "subtract":
			result = args.A - args.B
		case "multiply":
			result = args.A * args.B
		case "divide":
			if args.B == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			result = args.A / args.B
		default:
			return nil, fmt.Errorf("unknown operation: %s", args.Operation)
		}
		message := fmt.Sprintf("Result of %s: %.2f", args.Operation, result)
		return mcp.NewToolResponse(mcp.NewTextContent(message)), nil
	})
	if err != nil {
		panic(err)
	}

	err = server.RegisterTool("time", "Returns the current time", func(args TimeArgs) (*mcp.ToolResponse, error) {
		format := time.RFC3339
		if args.Format != "" {
			format = args.Format
		}
		message := time.Now().Format(format)
		return mcp.NewToolResponse(mcp.NewTextContent(message)), nil
	})
	if err != nil {
		panic(err)
	}

	err = server.RegisterTool("get_weather", "Get the weather forecast for temperature, wind speed and relative humidity", func(args WeatherArgs) (*mcp.ToolResponse, error) {
		url := fmt.Sprintf(
			"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,wind_speed_10m&hourly=temperature_2m,relative_humidity_2m,wind_speed_10m",
			args.Latitude, args.Longitude,
		)
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		output, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(string(output))), nil
	})
	if err != nil {
		panic(err)
	}

	// Prompts (uppercase, reverse)
	err = server.RegisterPrompt("uppercase", "Converts text to uppercase", func(args PromptArgs) (*mcp.PromptResponse, error) {
		text := strings.ToUpper(args.Input)
		return mcp.NewPromptResponse(
			"uppercase",
			mcp.NewPromptMessage(mcp.NewTextContent(text), mcp.RoleUser),
		), nil
	})
	if err != nil {
		panic(err)
	}

	err = server.RegisterPrompt("reverse", "Reverses the input text", func(args PromptArgs) (*mcp.PromptResponse, error) {
		runes := []rune(args.Input)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		text := string(runes)
		return mcp.NewPromptResponse(
			"reverse",
			mcp.NewPromptMessage(mcp.NewTextContent(text), mcp.RoleUser),
		), nil
	})
	if err != nil {
		panic(err)
	}

	// --------------------------
	// New File System Tools
	// --------------------------

	// read_file
	err = server.RegisterTool("read_file", "Reads the entire contents of a text file from disk", func(args ReadFileArgs) (*mcp.ToolResponse, error) {
		bytes, err := ioutil.ReadFile(args.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		return mcp.NewToolResponse(mcp.NewTextContent(string(bytes))), nil
	})
	if err != nil {
		panic(err)
	}

	// write_file
	err = server.RegisterTool("write_file", "Writes text content to a file (overwrites if it exists)", func(args WriteFileArgs) (*mcp.ToolResponse, error) {
		err := ioutil.WriteFile(args.Path, []byte(args.Content), 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to write file: %w", err)
		}
		return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Wrote file: %s", args.Path))), nil
	})
	if err != nil {
		panic(err)
	}

	// list_directory
	err = server.RegisterTool("list_directory", "Lists files and directories inside a given path", func(args ListDirectoryArgs) (*mcp.ToolResponse, error) {
		entries, err := ioutil.ReadDir(args.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}
		var lines []string
		for _, e := range entries {
			if e.IsDir() {
				lines = append(lines, "[DIR]  "+e.Name())
			} else {
				lines = append(lines, "[FILE] "+e.Name())
			}
		}
		return mcp.NewToolResponse(mcp.NewTextContent(strings.Join(lines, "\n"))), nil
	})
	if err != nil {
		panic(err)
	}

	// create_directory
	err = server.RegisterTool("create_directory", "Creates a directory (and any needed parent dirs)", func(args CreateDirectoryArgs) (*mcp.ToolResponse, error) {
		// 0755 permissions: owner can read/write/exec, others can read/exec
		err := os.MkdirAll(args.Path, 0755)
		if err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
		return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Directory created: %s", args.Path))), nil
	})
	if err != nil {
		panic(err)
	}

	// move_file (rename)
	err = server.RegisterTool("move_file", "Moves or renames a file/directory", func(args MoveFileArgs) (*mcp.ToolResponse, error) {
		err := os.Rename(args.Source, args.Destination)
		if err != nil {
			return nil, fmt.Errorf("failed to move/rename: %w", err)
		}
		return mcp.NewToolResponse(
			mcp.NewTextContent(
				fmt.Sprintf("Moved/renamed '%s' to '%s'", args.Source, args.Destination),
			),
		), nil
	})
	if err != nil {
		panic(err)
	}

	// --------------------------
	// Start the MCP server
	// --------------------------
	if err := server.Serve(); err != nil {
		panic(err)
	}

	// Keep the server running
	select {}
}
