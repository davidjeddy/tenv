/*
 *
 * Copyright 2024 tofuutils authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package asdfparser

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/tofuutils/tenv/v4/config"
	"github.com/tofuutils/tenv/v4/config/cmdconst"
)

//go:embed testdata/.tool-versions
var toolFileData []byte

// mockDisplayer implements loghelper.Displayer for testing
type mockDisplayer struct{}

func (m *mockDisplayer) Display(string)                          {}
func (m *mockDisplayer) Log(hclog.Level, string, ...interface{}) {}
func (m *mockDisplayer) IsDebug() bool                           { return false }
func (m *mockDisplayer) IsTrace() bool                           { return false }
func (m *mockDisplayer) Flush(bool)                              {}

func TestRetrieveTofuVersion(t *testing.T) {
	t.Parallel()
	testRetrieveVersion(t, cmdconst.OpentofuName, RetrieveTofuVersion)
}

func TestRetrieveTfVersion(t *testing.T) {
	t.Parallel()
	testRetrieveVersion(t, cmdconst.TerraformName, RetrieveTfVersion)
}

func TestRetrieveTgVersion(t *testing.T) {
	t.Parallel()
	testRetrieveVersion(t, cmdconst.TerragruntName, RetrieveTgVersion)
}

func TestRetrieveAtmosVersion(t *testing.T) {
	t.Parallel()
	testRetrieveVersion(t, cmdconst.AtmosName, RetrieveAtmosVersion)
}

func testRetrieveVersion(t *testing.T, toolName string, retrieveFunc func(string, *config.Config) (string, error)) {
	tests := []struct {
		name           string
		content        string
		expectedResult string
		expectError    bool
	}{
		{
			name:           "valid version",
			content:        toolName + " 1.0.0",
			expectedResult: "1.0.0",
		},
		{
			name:           "version with comment",
			content:        toolName + " 1.0.0 # comment",
			expectedResult: "1.0.0",
		},
		{
			name:           "version with inline comment",
			content:        toolName + " 1.0.0#comment",
			expectedResult: "1.0.0",
		},
		{
			name:           "multiple tools",
			content:        "nodejs 14.0.0\n" + toolName + " 1.0.0\npython 3.8.0",
			expectedResult: "1.0.0",
		},
		{
			name:           "empty file",
			content:        "",
			expectedResult: "",
		},
		{
			name:           "comments only",
			content:        "# comment\n# another comment",
			expectedResult: "",
		},
		{
			name:           "tool not found",
			content:        "nodejs 14.0.0\npython 3.8.0",
			expectedResult: "",
		},
		{
			name:           "multiple versions",
			content:        toolName + " 1.0.0\n" + toolName + " 2.0.0",
			expectedResult: "1.0.0",
		},
		{
			name:           "version with spaces",
			content:        toolName + "    1.0.0    ",
			expectedResult: "1.0.0",
		},
		{
			name:           "version with tabs",
			content:        toolName + "\t1.0.0\t",
			expectedResult: "1.0.0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup temp directory
			tempDir := t.TempDir()
			filePath := filepath.Join(tempDir, ToolFileName)

			// Create test file
			err := os.WriteFile(filePath, []byte(tt.content), 0600)
			if err != nil {
				t.Fatal(err)
			}

			// Create config
			conf := &config.Config{
				Displayer: &mockDisplayer{},
			}

			// Run test
			result, err := retrieveFunc(filePath, conf)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if result != tt.expectedResult {
				t.Errorf("expected %s but got %s", tt.expectedResult, result)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	// Setup temp directory
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ToolFileName)

	// Create test file
	content := "terraform 1.0.0\nterragrunt 1.2.0\nopentofu 1.3.0\natmos 1.4.0"
	err := os.WriteFile(filePath, []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Create config
	conf := &config.Config{
		Displayer: &mockDisplayer{},
	}

	// Number of concurrent goroutines
	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Run concurrent tests
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// Test all version retrieval functions
			funcs := []struct {
				name string
				fn   func(string, *config.Config) (string, error)
			}{
				{"RetrieveTfVersion", RetrieveTfVersion},
				{"RetrieveTgVersion", RetrieveTgVersion},
				{"RetrieveTofuVersion", RetrieveTofuVersion},
				{"RetrieveAtmosVersion", RetrieveAtmosVersion},
			}

			for _, f := range funcs {
				result, err := f.fn(filePath, conf)
				if err != nil {
					t.Error(err)
					return
				}
				expected := ""
				switch f.name {
				case "RetrieveTfVersion":
					expected = "1.0.0"
				case "RetrieveTgVersion":
					expected = "1.2.0"
				case "RetrieveTofuVersion":
					expected = "1.3.0"
				case "RetrieveAtmosVersion":
					expected = "1.4.0"
				}
				if result != expected {
					t.Errorf("for %s, expected %s but got %s", f.name, expected, result)
				}
			}
		}()
	}

	wg.Wait()
}

func TestFileErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(string) error
		expectError bool
	}{
		{
			name: "non-existent file",
			setup: func(dir string) error {
				return nil // No setup needed, file doesn't exist
			},
			expectError: false, // Should return empty string, not error
		},
		{
			name: "unreadable file",
			setup: func(dir string) error {
				filePath := filepath.Join(dir, ToolFileName)
				if err := os.WriteFile(filePath, []byte("terraform 1.0.0"), 0600); err != nil {
					return err
				}
				return os.Chmod(filePath, 0000)
			},
			expectError: true,
		},
		{
			name: "directory instead of file",
			setup: func(dir string) error {
				filePath := filepath.Join(dir, ToolFileName)
				return os.Mkdir(filePath, 0700)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup temp directory
			tempDir := t.TempDir()

			// Apply setup
			if err := tt.setup(tempDir); err != nil {
				t.Fatal(err)
			}

			// Create config
			conf := &config.Config{
				Displayer: &mockDisplayer{},
			}

			// Run test
			filePath := filepath.Join(tempDir, ToolFileName)
			_, err := RetrieveTfVersion(filePath, conf)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestParseVersionFromToolFileReader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		content        string
		toolName       string
		expectedResult string
	}{
		{
			name:           "valid version",
			content:        "terraform 1.0.0",
			toolName:       "terraform",
			expectedResult: "1.0.0",
		},
		{
			name:           "version with comment",
			content:        "terraform 1.0.0 # comment",
			toolName:       "terraform",
			expectedResult: "1.0.0",
		},
		{
			name:           "version with inline comment",
			content:        "terraform 1.0.0#comment",
			toolName:       "terraform",
			expectedResult: "1.0.0",
		},
		{
			name:           "multiple tools",
			content:        "nodejs 14.0.0\nterraform 1.0.0\npython 3.8.0",
			toolName:       "terraform",
			expectedResult: "1.0.0",
		},
		{
			name:           "empty content",
			content:        "",
			toolName:       "terraform",
			expectedResult: "",
		},
		{
			name:           "comments only",
			content:        "# comment\n# another comment",
			toolName:       "terraform",
			expectedResult: "",
		},
		{
			name:           "tool not found",
			content:        "nodejs 14.0.0\npython 3.8.0",
			toolName:       "terraform",
			expectedResult: "",
		},
		{
			name:           "multiple versions",
			content:        "terraform 1.0.0\nterraform 2.0.0",
			toolName:       "terraform",
			expectedResult: "1.0.0",
		},
		{
			name:           "version with spaces",
			content:        "terraform    1.0.0    ",
			toolName:       "terraform",
			expectedResult: "1.0.0",
		},
		{
			name:           "version with tabs",
			content:        "terraform\t1.0.0\t",
			toolName:       "terraform",
			expectedResult: "1.0.0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create reader
			reader := strings.NewReader(tt.content)

			// Create displayer
			displayer := &mockDisplayer{}

			// Run test
			result := parseVersionFromToolFileReader("test.tool-versions", reader, tt.toolName, displayer)

			if result != tt.expectedResult {
				t.Errorf("expected %s but got %s", tt.expectedResult, result)
			}
		})
	}
}

func TestFileEncodings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		content        []byte
		expectedResult string
		expectError    bool
	}{
		{
			name:           "UTF-8",
			content:        []byte("terraform 1.0.0"),
			expectedResult: "1.0.0",
		},
		{
			name:           "UTF-8 with BOM",
			content:        append([]byte{0xEF, 0xBB, 0xBF}, []byte("terraform 1.0.0")...),
			expectedResult: "1.0.0",
		},
		{
			name:        "UTF-16",
			content:     append([]byte{0xFF, 0xFE}, []byte("terraform 1.0.0")...),
			expectError: true,
		},
		{
			name:           "ASCII",
			content:        []byte("terraform 1.0.0"),
			expectedResult: "1.0.0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup temp directory
			tempDir := t.TempDir()
			filePath := filepath.Join(tempDir, ToolFileName)

			// Create test file
			err := os.WriteFile(filePath, tt.content, 0600)
			if err != nil {
				t.Fatal(err)
			}

			// Create config
			conf := &config.Config{
				Displayer: &mockDisplayer{},
			}

			// Run test
			result, err := RetrieveTfVersion(filePath, conf)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if result != tt.expectedResult {
				t.Errorf("expected %s but got %s", tt.expectedResult, result)
			}
		})
	}
}

func TestLargeFiles(t *testing.T) {
	t.Parallel()

	// Setup temp directory
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ToolFileName)

	// Create a large file with version constraint
	content := make([]byte, 10*1024*1024) // 10MB
	copy(content, []byte("terraform 1.0.0"))

	// Create test file
	err := os.WriteFile(filePath, content, 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Create config
	conf := &config.Config{
		Displayer: &mockDisplayer{},
	}

	// Run test
	result, err := RetrieveTfVersion(filePath, conf)
	if err != nil {
		t.Fatal(err)
	}

	if result != "1.0.0" {
		t.Errorf("expected 1.0.0 but got %s", result)
	}
}

func TestSymbolicLinks(t *testing.T) {
	t.Parallel()

	// Setup temp directory
	tempDir := t.TempDir()
	originalPath := filepath.Join(tempDir, "original.tool-versions")
	linkPath := filepath.Join(tempDir, ToolFileName)

	// Create original file
	content := "terraform 1.0.0"
	err := os.WriteFile(originalPath, []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Create symbolic link
	err = os.Symlink(originalPath, linkPath)
	if err != nil {
		t.Fatal(err)
	}

	// Create config
	conf := &config.Config{
		Displayer: &mockDisplayer{},
	}

	// Run test
	result, err := RetrieveTfVersion(linkPath, conf)
	if err != nil {
		t.Fatal(err)
	}

	if result != "1.0.0" {
		t.Errorf("expected 1.0.0 but got %s", result)
	}
}

func TestMultipleFiles(t *testing.T) {
	t.Parallel()

	// Setup temp directory
	tempDir := t.TempDir()

	// Create multiple files with different version constraints
	files := []struct {
		name    string
		content string
	}{
		{
			name:    ToolFileName,
			content: "terraform 1.0.0",
		},
		{
			name:    "other.tool-versions",
			content: "terraform 1.1.0",
		},
		{
			name:    "config.tool-versions",
			content: "nodejs 14.0.0",
		},
	}

	// Create config
	conf := &config.Config{
		Displayer: &mockDisplayer{},
	}

	// Create and test each file
	for _, file := range files {
		filePath := filepath.Join(tempDir, file.name)
		err := os.WriteFile(filePath, []byte(file.content), 0600)
		if err != nil {
			t.Fatal(err)
		}

		result, err := RetrieveTfVersion(filePath, conf)
		if err != nil {
			t.Fatal(err)
		}

		expected := ""
		if file.name != "config.tool-versions" {
			expected = "1.0.0"
			if file.name == "other.tool-versions" {
				expected = "1.1.0"
			}
		}

		if result != expected {
			t.Errorf("for file %s, expected %s but got %s", file.name, expected, result)
		}
	}
}
