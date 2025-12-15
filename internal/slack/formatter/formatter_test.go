package formatter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple text",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "Text with quoted strings - no longer converted",
			input:    "Created on \"2020-11-17T05:07:52Z\" or \"2020-11-17T05:07:54Z\"",
			expected: "Created on \"2020-11-17T05:07:52Z\" or \"2020-11-17T05:07:54Z\"", // Quotes preserved as-is
		},
		{
			name:     "Header conversion",
			input:    "### Step-by-Step Instructions",
			expected: "*Step-by-Step Instructions*",
		},
		{
			name:     "Multiple headers",
			input:    "# Main Title\n## Section\n### Subsection",
			expected: "*Main Title*\n*Section*\n*Subsection*",
		},
		{
			name:     "Markdown link conversion",
			input:    "Check the [documentation](https://github.com/prometheus/snmp_exporter/tree/main)",
			expected: "Check the <https://github.com/prometheus/snmp_exporter/tree/main|documentation>",
		},
		{
			name:     "Bare URL conversion",
			input:    "Visit https://github.com/prometheus/snmp_exporter for more info",
			expected: "Visit <https://github.com/prometheus/snmp_exporter> for more info",
		},
		// Add more test cases as needed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMarkdown(tt.input)
			if result != tt.expected {
				t.Errorf("FormatMarkdown() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConvertQuotedStringsToCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "No quoted strings",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "Single quoted string",
			input:    "Namespace \"kube-system\" is a system namespace",
			expected: "Namespace `kube-system` is a system namespace",
		},
		{
			name:     "Multiple quoted strings",
			input:    "All of these were created on \"2020-11-17T05:07:52Z\" or \"2020-11-17T05:07:54Z\". Among them, \"kube-node-lease\", \"kube-public\", and \"kube-system\" share the exact same creation timestamp: \"2020-11-17T05:07:52Z\", making them the oldest namespaces in your cluster.",
			expected: "All of these were created on `2020-11-17T05:07:52Z` or `2020-11-17T05:07:54Z`. Among them, `kube-node-lease`, `kube-public`, and `kube-system` share the exact same creation timestamp: `2020-11-17T05:07:52Z`, making them the oldest namespaces in your cluster.",
		},
		{
			name:     "Escaped quotes",
			input:    "The command \"echo \\\"Hello\\\"\" prints Hello",
			expected: "The command `echo \\\"Hello\\\"` prints Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertQuotedStringsToCode(tt.input)
			if result != tt.expected {
				t.Errorf("ConvertQuotedStringsToCode() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCreateBlockMessage(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		blockOptions BlockOptions
		expectBlocks bool
	}{
		{
			name: "Simple block with header",
			text: "This is a test message",
			blockOptions: BlockOptions{
				HeaderText: "Test Header",
			},
			expectBlocks: true,
		},
		{
			name: "Block with fields",
			text: "This is a test message with fields",
			blockOptions: BlockOptions{
				HeaderText: "Test Header",
				Fields: []Field{
					{Title: "Status", Value: "Success"},
					{Title: "Duration", Value: "5m 32s"},
				},
			},
			expectBlocks: true,
		},
		{
			name: "Block with actions",
			text: "This is a test message with actions",
			blockOptions: BlockOptions{
				HeaderText: "Test Header",
				Actions: []Action{
					{Text: "View Details", URL: "http://example.com"},
				},
			},
			expectBlocks: true,
		},
		{
			name: "Long text split into multiple blocks",
			text: strings.Repeat("This is a very long message that exceeds the Slack block limit. ", 50), // ~3250 chars
			blockOptions: BlockOptions{
				HeaderText: "Long Message Test",
			},
			expectBlocks: true,
		},
		{
			name: "Code block spanning chunk boundary",
			text: strings.Repeat("This is some text. ", 100) + "```\n" + strings.Repeat("line of code\n", 20) + "```\n" + strings.Repeat("This is more text. ", 100), // Ensure code block crosses chunk boundary
			blockOptions: BlockOptions{
				HeaderText: "Code Block Test",
			},
			expectBlocks: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateBlockMessage(tt.text, tt.blockOptions)

			// Verify it's valid JSON
			var parsed map[string]interface{}
			err := json.Unmarshal([]byte(result), &parsed)
			if err != nil {
				t.Errorf("CreateBlockMessage() produced invalid JSON: %v", err)
				return
			}

			// Check if blocks exist
			blocks, ok := parsed["blocks"]
			if tt.expectBlocks && (!ok || blocks == nil) {
				t.Errorf("CreateBlockMessage() did not produce blocks")
			}
		})
	}
}

func TestDetectMessageType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected MessageType
	}{
		{
			name:     "Plain text",
			input:    "Hello world",
			expected: PlainText,
		},
		{
			name:     "Markdown text",
			input:    "Hello *bold* _italic_ world",
			expected: MarkdownText,
		},
		{
			name:     "Markdown with headers",
			input:    "### Step-by-Step Instructions\n\nHere are the steps:",
			expected: MarkdownText,
		},
		{
			name: "JSON Block",
			input: `{
				"text": "Hello world",
				"blocks": [
					{
						"type": "section",
						"text": {
							"type": "mrkdwn",
							"text": "Hello world"
						}
					}
				]
			}`,
			expected: JSONBlock,
		},
		{
			name: "Structured data",
			input: `Status: Success
Duration: 5m 32s
Result: Passed`,
			expected: StructuredData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectMessageType(tt.input)
			if result != tt.expected {
				t.Errorf("DetectMessageType() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractStructuredData(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name: "Simple key-value pairs",
			input: `Status: Success
Duration: 5m 32s
Result: Passed`,
			expected: map[string]string{
				"Status":   "Success",
				"Duration": "5m 32s",
				"Result":   "Passed",
			},
		},
		{
			name:     "No structured data",
			input:    "Hello world",
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractStructuredData(tt.input)

			// Check if maps have the same length
			if len(result) != len(tt.expected) {
				t.Errorf("ExtractStructuredData() returned map with length %d, want %d", len(result), len(tt.expected))
				return
			}

			// Check if all expected keys are present with correct values
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("ExtractStructuredData() for key %s = %v, want %v", k, result[k], v)
				}
			}
		})
	}
}

func TestFormatStructuredData(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectBlocks bool
	}{
		{
			name: "Structured data",
			input: `Status: Success
Duration: 5m 32s
Result: Passed`,
			expectBlocks: true,
		},
		{
			name:         "Non-structured data",
			input:        "Hello world",
			expectBlocks: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatStructuredData(tt.input)

			// Check if it's JSON for structured data
			if tt.expectBlocks {
				var parsed map[string]interface{}
				err := json.Unmarshal([]byte(result), &parsed)
				if err != nil {
					t.Errorf("FormatStructuredData() produced invalid JSON: %v", err)
					return
				}

				// Check if blocks exist
				blocks, ok := parsed["blocks"]
				if !ok || blocks == nil {
					t.Errorf("FormatStructuredData() did not produce blocks")
				}
			} else {
				// For non-structured data, it should return the original content
				if result != tt.input {
					t.Errorf("FormatStructuredData() = %v, want %v", result, tt.input)
				}
			}
		})
	}
}

func TestCreateResponseHelpers(t *testing.T) {
	tests := []struct {
		name     string
		function func(string, string) string
		content  string
		traceURL string
	}{
		{
			name:     "Success response with trace",
			function: CreateSuccessResponse,
			content:  "Operation completed successfully!",
			traceURL: "https://trace.example.com/abc123",
		},
		{
			name:     "Error response with trace",
			function: CreateErrorResponse,
			content:  "An error occurred",
			traceURL: "https://trace.example.com/error456",
		},
		{
			name:     "Info response with trace",
			function: CreateInfoResponse,
			content:  "Here's some information",
			traceURL: "https://trace.example.com/info789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.function(tt.content, tt.traceURL)

			// Verify it's valid JSON
			var parsed map[string]interface{}
			err := json.Unmarshal([]byte(result), &parsed)
			if err != nil {
				t.Errorf("Helper produced invalid JSON: %v", err)
				return
			}

			// Check if blocks exist
			blocks, ok := parsed["blocks"]
			if !ok || blocks == nil {
				t.Errorf("Helper did not produce blocks")
			}
		})
	}
}

func TestBlockKitMarkdownRendering(t *testing.T) {
	// Test that inline code (backticks) are preserved in mrkdwn sections
	testText := "Use the `vault/jwt_role` module for configuration. See `documentation/github-config-new.pdf` for details."

	blockJSON := CreateBlockMessage(testText, BlockOptions{
		HeaderText: "Test Message",
	})

	// Parse the JSON
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(blockJSON), &parsed)
	if err != nil {
		t.Fatalf("Failed to parse block JSON: %v", err)
	}

	// Check blocks
	blocks, ok := parsed["blocks"].([]interface{})
	if !ok {
		t.Fatal("No blocks found in message")
	}

	// Find the section block
	var sectionFound bool
	for _, block := range blocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}

		if blockMap["type"] == "section" {
			sectionFound = true

			// Check the text object
			textObj, ok := blockMap["text"].(map[string]interface{})
			if !ok {
				t.Error("Section block missing text object")
				continue
			}

			// Verify it's mrkdwn type
			textType, _ := textObj["type"].(string)
			if textType != "mrkdwn" {
				t.Errorf("Expected text type 'mrkdwn', got '%s'", textType)
			}

			// Verify backticks are preserved in the text
			textContent, _ := textObj["text"].(string)
			if !strings.Contains(textContent, "`vault/jwt_role`") {
				t.Error("Backticks not preserved in section text")
			}
			if !strings.Contains(textContent, "`documentation/github-config-new.pdf`") {
				t.Error("Backticks for file path not preserved in section text")
			}
		}
	}

	if !sectionFound {
		t.Error("No section block found in message")
	}
}
