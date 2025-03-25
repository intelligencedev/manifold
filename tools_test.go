// File: tools_test.go
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Save the original HTTP client's Do method so we can restore it later.
var originalHTTPGet = http.DefaultClient.Do

func TestMain(m *testing.M) {
	// Override RunCommand globally to avoid calling actual OS commands during tests.
	RunCommand = func(name, dir string, args ...string) (string, error) {
		return "mocked output", nil
	}

	// Override HTTP requests by default so real external calls don't run.
	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			// Default mock response.
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"mock":"response"}`)),
				Header:     make(http.Header),
			}
		}),
	}

	code := m.Run()

	// Restore the original HTTP client transport.
	http.DefaultClient = &http.Client{Transport: nil}

	os.Exit(code)
}

// roundTripFunc is a helper to mock http.Client Transport behavior.
type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

// -----------------------
// Test: hello tool
// -----------------------
func TestHelloTool(t *testing.T) {
	t.Run("valid name", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"name":"Alice"}`)
		resp, err := ExecuteToolByName("hello", rawArgs)
		require.NoError(t, err)
		assert.Equal(t, "Hello, Alice!", resp)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"name":123`)
		resp, err := ExecuteToolByName("hello", rawArgs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid arguments for hello tool")
		assert.Empty(t, resp)
	})
}

// -----------------------
// Test: calculate tool
// -----------------------
func TestCalculateTool(t *testing.T) {
	t.Run("add", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"operation":"add","a":5,"b":3}`)
		resp, err := ExecuteToolByName("calculate", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "Result of add: 8.00")
	})

	t.Run("subtract", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"operation":"subtract","a":5,"b":3}`)
		resp, err := ExecuteToolByName("calculate", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "Result of subtract: 2.00")
	})

	t.Run("multiply", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"operation":"multiply","a":5,"b":3}`)
		resp, err := ExecuteToolByName("calculate", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "Result of multiply: 15.00")
	})

	t.Run("divide success", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"operation":"divide","a":6,"b":2}`)
		resp, err := ExecuteToolByName("calculate", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "Result of divide: 3.00")
	})

	t.Run("divide by zero", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"operation":"divide","a":6,"b":0}`)
		resp, err := ExecuteToolByName("calculate", rawArgs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "division by zero")
		assert.Empty(t, resp)
	})

	t.Run("unknown operation", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"operation":"modulo","a":6,"b":2}`)
		resp, err := ExecuteToolByName("calculate", rawArgs)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown operation: modulo")
		assert.Empty(t, resp)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"operation":"add",bad}`)
		resp, err := ExecuteToolByName("calculate", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
	})
}

// -----------------------
// Test: time tool
// -----------------------
func TestTimeTool(t *testing.T) {
	t.Run("no format provided", func(t *testing.T) {
		rawArgs := json.RawMessage(`{}`)
		resp, err := ExecuteToolByName("time", rawArgs)
		require.NoError(t, err)
		_, parseErr := time.Parse(time.RFC3339, resp)
		assert.NoError(t, parseErr)
	})

	t.Run("custom format provided", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"format":"2006-01-02"}`)
		resp, err := ExecuteToolByName("time", rawArgs)
		require.NoError(t, err)
		_, parseErr := time.Parse("2006-01-02", resp)
		assert.NoError(t, parseErr)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"format":123abc}`)
		resp, err := ExecuteToolByName("time", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
	})
}

// -----------------------
// Test: weather tool
// -----------------------
func TestWeatherTool(t *testing.T) {
	t.Run("valid call to weather API", func(t *testing.T) {
		oldClient := http.DefaultClient
		defer func() { http.DefaultClient = oldClient }()

		mockTransport := roundTripFunc(func(req *http.Request) *http.Response {
			require.Contains(t, req.URL.String(), "api.open-meteo.com")
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"weather":"mocked"}`)),
				Header:     make(http.Header),
			}
		})
		http.DefaultClient = &http.Client{Transport: mockTransport}

		rawArgs := json.RawMessage(`{"latitude":37.7749,"longitude":-122.4194}`)
		resp, err := ExecuteToolByName("weather", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, `"weather":"mocked"`)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"latitude":"???"}`)
		resp, err := ExecuteToolByName("weather", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
	})
}

// -----------------------
// Test: read_file tool
// -----------------------
func TestReadFileTool(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-readfile-*.txt")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		content := "Hello, file!"
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)
		tmpFile.Close()

		rawArgs := json.RawMessage(`{"path":"` + tmpFile.Name() + `"}`)
		resp, err := ExecuteToolByName("read_file", rawArgs)
		require.NoError(t, err)
		assert.Equal(t, content, resp)
	})

	t.Run("file not found", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"path":"/nonexistent/path.txt"}`)
		resp, err := ExecuteToolByName("read_file", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
	})
}

// -----------------------
// Test: write_file tool
// -----------------------
func TestWriteFileTool(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-writefile")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		targetPath := filepath.Join(tmpDir, "output.txt")
		content := "Hello from test"
		rawArgs := json.RawMessage(`{"path":"` + targetPath + `","content":"` + content + `"}`)

		resp, err := ExecuteToolByName("write_file", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "Wrote file")

		bytes, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, content, string(bytes))
	})

	t.Run("invalid JSON", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"path":"somefile.txt","content":123abc}`)
		resp, err := ExecuteToolByName("write_file", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
	})
}

// -----------------------
// Test: list_directory tool
// -----------------------
func TestListDirectoryTool(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-listdir")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		err = os.WriteFile(filepath.Join(tmpDir, "test1.txt"), []byte("test1"), 0644)
		require.NoError(t, err)
		err = os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `"}`)
		resp, err := ExecuteToolByName("list_directory", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "[FILE] test1.txt")
		assert.Contains(t, resp, "[DIR]  subdir")
	})

	t.Run("dir not found", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"path":"/nonexistent/dir"}`)
		resp, err := ExecuteToolByName("list_directory", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
	})
}

// -----------------------
// Test: create_directory tool
// -----------------------
func TestCreateDirectoryTool(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-createdir")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		newDir := filepath.Join(tmpDir, "subfolder")
		rawArgs := json.RawMessage(`{"path":"` + newDir + `"}`)
		resp, err := ExecuteToolByName("create_directory", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "Directory created:")

		info, err := os.Stat(newDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

// -----------------------
// Test: move_file tool
// -----------------------
func TestMoveFileTool(t *testing.T) {
	t.Run("rename file", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-movefile")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		oldPath := filepath.Join(tmpDir, "old.txt")
		newPath := filepath.Join(tmpDir, "new.txt")

		err = os.WriteFile(oldPath, []byte("some data"), 0644)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"source":"` + oldPath + `","destination":"` + newPath + `"}`)
		resp, err := ExecuteToolByName("move_file", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "Moved/renamed")

		_, err = os.Stat(oldPath)
		assert.True(t, os.IsNotExist(err))

		_, err = os.Stat(newPath)
		assert.NoError(t, err)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"source":123}`)
		resp, err := ExecuteToolByName("move_file", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
	})
}

// -----------------------
// Test: git_init tool
// -----------------------
func TestGitInitTool(t *testing.T) {
	t.Run("success with mock runCommand", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-gitinit")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `"}`)
		resp, err := ExecuteToolByName("git_init", rawArgs)
		require.NoError(t, err)
		// Expecting the output from our (global) mock – see TestMain.
		assert.Contains(t, resp, "mocked output")
	})

	t.Run("path not a directory", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-gitinit-file")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		rawArgs := json.RawMessage(`{"path":"` + tmpFile.Name() + `"}`)
		resp, err := ExecuteToolByName("git_init", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
	})
}

// -----------------------
// Test: git_status tool
// -----------------------
func TestGitStatusTool(t *testing.T) {
	t.Run("valid git repo (mocked)", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-gitstatus")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		err = os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `"}`)
		resp, err := ExecuteToolByName("git_status", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})

	t.Run("not a git repo", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-gitstatus2")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `"}`)
		resp, err := ExecuteToolByName("git_status", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
		assert.Contains(t, err.Error(), "this path does not appear to be a Git repo")
	})
}

// -----------------------
// Test: git_add tool
// -----------------------
func TestGitAddTool(t *testing.T) {
	t.Run("valid git repo with fileList", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-gitadd")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		err = os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `","fileList":["file1.txt","file2.txt"]}`)
		resp, err := ExecuteToolByName("git_add", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})

	t.Run("valid git repo no fileList => add .", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-gitadd2")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		err = os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `"}`)
		resp, err := ExecuteToolByName("git_add", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})

	t.Run("not a git repo", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-gitadd3")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `","fileList":["file1.txt"]}`)
		resp, err := ExecuteToolByName("git_add", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
		assert.Contains(t, err.Error(), "this path does not appear to be a Git repo")
	})
}

// -----------------------
// Test: git_commit tool
// -----------------------
func TestGitCommitTool(t *testing.T) {
	t.Run("valid commit", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-gitcommit")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		err = os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `","message":"Initial commit"}`)
		resp, err := ExecuteToolByName("git_commit", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})

	t.Run("empty message", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-gitcommit2")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		err = os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `","message":""}`)
		resp, err := ExecuteToolByName("git_commit", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
		assert.Contains(t, err.Error(), "commit message cannot be empty")
	})
}

// -----------------------
// Test: git_pull tool
// -----------------------
func TestGitPullTool(t *testing.T) {
	t.Run("valid git repo", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-gitpull")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		err = os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `"}`)
		resp, err := ExecuteToolByName("git_pull", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})
}

// -----------------------
// Test: git_push tool
// -----------------------
func TestGitPushTool(t *testing.T) {
	t.Run("valid git repo", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-gitpush")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		err = os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `"}`)
		resp, err := ExecuteToolByName("git_push", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})
}

// -----------------------
// Test: search_files tool
// -----------------------
func TestSearchFilesTool(t *testing.T) {
	t.Run("find matches", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-searchfiles")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		err = os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("hello world"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("goodbye"), 0644)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `","pattern":"world"}`)
		resp, err := ExecuteToolByName("search_files", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "file1.txt")
		assert.NotContains(t, resp, "file2.txt")
	})

	t.Run("no matches", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-searchfiles2")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `","pattern":"xxx"}`)
		resp, err := ExecuteToolByName("search_files", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "No files found matching the pattern.")
	})
}

// -----------------------
// Test: delete_file tool
// -----------------------
func TestDeleteFileTool(t *testing.T) {
	t.Run("delete file non-recursive", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-deletefile")
		require.NoError(t, err)
		path := tmpFile.Name()
		tmpFile.Close()

		rawArgs := json.RawMessage(`{"path":"` + path + `"}`)
		resp, err := ExecuteToolByName("delete_file", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "Deleted:")

		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("delete directory recursive", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-deletedir")
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("test"), 0644)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `","recursive":true}`)
		resp, err := ExecuteToolByName("delete_file", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "Deleted:")

		_, err = os.Stat(tmpDir)
		assert.True(t, os.IsNotExist(err))
	})
}

// -----------------------
// Test: copy_file tool
// -----------------------
func TestCopyFileTool(t *testing.T) {
	t.Run("copy single file", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-copyfile")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		srcFile := filepath.Join(tmpDir, "src.txt")
		dstFile := filepath.Join(tmpDir, "dst.txt")

		err = os.WriteFile(srcFile, []byte("hello"), 0644)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"source":"` + srcFile + `","destination":"` + dstFile + `"}`)
		resp, err := ExecuteToolByName("copy_file", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "Copied from")

		data, err := os.ReadFile(dstFile)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(data))
	})

	t.Run("copy directory without recursive => error", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-copydir")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		srcDir := filepath.Join(tmpDir, "srcdir")
		dstDir := filepath.Join(tmpDir, "dstdir")
		err = os.Mkdir(srcDir, 0755)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"source":"` + srcDir + `","destination":"` + dstDir + `"}`)
		resp, err := ExecuteToolByName("copy_file", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
		assert.Contains(t, err.Error(), "set recursive to true to copy directories")
	})
}

// -----------------------
// Test: git_clone tool
// -----------------------
func TestGitCloneTool(t *testing.T) {
	t.Run("basic clone mock", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"repoUrl":"https://github.com/example/repo.git","path":"/tmp/repo"}`)
		resp, err := ExecuteToolByName("git_clone", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})
}

// -----------------------
// Test: git_checkout tool
// -----------------------
func TestGitCheckoutTool(t *testing.T) {
	t.Run("checkout existing branch", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-gitcheckout")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)
		err = os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755)
		require.NoError(t, err)

		rawArgs := json.RawMessage(`{"path":"` + tmpDir + `","branch":"main","createNew":false}`)
		resp, err := ExecuteToolByName("git_checkout", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})
}

// -----------------------
// Test: run_shell_command tool
// -----------------------
func TestRunShellCommandTool(t *testing.T) {
	t.Run("valid command", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"command":["ls","-la"],"dir":"/tmp"}`)
		resp, err := ExecuteToolByName("run_shell_command", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})

	t.Run("empty command => error", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"command":[],"dir":"/tmp"}`)
		resp, err := ExecuteToolByName("run_shell_command", rawArgs)
		require.Error(t, err)
		assert.Empty(t, resp)
		assert.Contains(t, err.Error(), "empty command")
	})
}

// -----------------------
// Test: go_build tool
// -----------------------
func TestGoBuildTool(t *testing.T) {
	t.Run("success mock build", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"path":"/tmp/goproject"}`)
		resp, err := ExecuteToolByName("go_build", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})
}

// -----------------------
// Test: go_test tool
// -----------------------
func TestGoTestTool(t *testing.T) {
	t.Run("success mock test", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"path":"/tmp/goproject"}`)
		resp, err := ExecuteToolByName("go_test", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})
}

// -----------------------
// Test: format_go_code tool
// -----------------------
func TestFormatGoCodeTool(t *testing.T) {
	t.Run("success mock go fmt", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"path":"/tmp/goproject"}`)
		resp, err := ExecuteToolByName("format_go_code", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})
}

// -----------------------
// Test: lint_code tool
// -----------------------
func TestLintCodeTool(t *testing.T) {
	t.Run("with linterName", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"path":"/tmp/goproject","linterName":"golangci-lint"}`)
		resp, err := ExecuteToolByName("lint_code", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})

	t.Run("no linterName => defaults to golangci-lint", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"path":"/tmp/goproject"}`)
		resp, err := ExecuteToolByName("lint_code", rawArgs)
		require.NoError(t, err)
		assert.Contains(t, resp, "mocked output")
	})
}

// -----------------------
// Test: GetAllTools
// -----------------------
func TestGetAllTools(t *testing.T) {
	tools := GetAllTools()
	require.NotEmpty(t, tools)

	var foundHello, foundCalculate bool
	for _, tinfo := range tools {
		switch tinfo.Name {
		case "hello":
			foundHello = true
		case "calculate":
			foundCalculate = true
		}
	}
	assert.True(t, foundHello)
	assert.True(t, foundCalculate)
}

// -----------------------
// Test: ExecuteToolByName
// -----------------------
func TestExecuteToolByName(t *testing.T) {
	t.Run("existing tool", func(t *testing.T) {
		resp, err := ExecuteToolByName("hello", json.RawMessage(`{"name":"World"}`))
		require.NoError(t, err)
		assert.Equal(t, "Hello, World!", resp)
	})

	t.Run("unknown tool", func(t *testing.T) {
		resp, err := ExecuteToolByName("does_not_exist", json.RawMessage(`{}`))
		require.Error(t, err)
		assert.Empty(t, resp)
		assert.Contains(t, err.Error(), "unrecognized tool name")
	})
}
