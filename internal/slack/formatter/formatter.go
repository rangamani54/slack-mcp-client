// Package formatter provides utilities for formatting messages for Slack
// It supports both mrkdwn (Markdown) and Block Kit structures
package formatter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

// MessageFormat represents the format of a message to be sent to Slack
type MessageFormat int

const (
	// TextFormat represents a simple text message with mrkdwn formatting
	TextFormat MessageFormat = iota
	// BlockFormat represents a message with Block Kit structures
	BlockFormat
)

// FormatOptions contains options for formatting a message
type FormatOptions struct {
	Format     MessageFormat
	ThreadTS   string
	EscapeText bool
}

// DefaultOptions returns the default formatting options
func DefaultOptions() FormatOptions {
	return FormatOptions{
		Format:     TextFormat,
		ThreadTS:   "",
		EscapeText: true,
	}
}

// BlockOptions contains options for Block Kit messages
type BlockOptions struct {
	HeaderText          string
	HeaderEmoji         string // Optional emoji for header (e.g., "🤖", "✅", "📊")
	Fields              []Field
	Actions             []Action
	FooterMrkdwn        string
	FooterIcon          string // Optional emoji for footer (e.g., "🔗", "📎", "ℹ️")
	DividerBeforeFooter bool
	ShowTimestamp       bool // Add timestamp to footer
}

// Field represents a field in a section block
type Field struct {
	Title string
	Value string
}

// Action represents an action button
type Action struct {
	Text string
	URL  string
}

// FormatMessage formats a message for Slack based on the provided options
func FormatMessage(text string, options FormatOptions) []slack.MsgOption {
	var msgOptions []slack.MsgOption

	if options.ThreadTS != "" {
		msgOptions = append(msgOptions, slack.MsgOptionTS(options.ThreadTS))
	}

	switch options.Format {
	case BlockFormat:
		// Parse the text as JSON Block Kit format
		var blockMessage struct {
			Text   string        `json:"text"`
			Blocks []interface{} `json:"blocks"`
		}

		if err := json.Unmarshal([]byte(text), &blockMessage); err == nil {
			// Successfully parsed as Block Kit JSON
			var blocks slack.Blocks
			// Convert the generic blocks to slack.Block objects
			for _, block := range blockMessage.Blocks {
				blockJSON, err := json.Marshal(block)
				if err != nil {
					continue
				}

				// Parse the block based on its type
				var blockMap map[string]interface{}
				if err := json.Unmarshal(blockJSON, &blockMap); err != nil {
					continue
				}

				blockType, ok := blockMap["type"].(string)
				if !ok {
					continue
				}

				var slackBlock slack.Block
				switch blockType {
				case "section":
					// Manually construct section to ensure mrkdwn type is preserved
					var section slack.SectionBlock
					if err := json.Unmarshal(blockJSON, &section); err == nil {
						// Check if there's a text object and ensure it's mrkdwn type
						if textObj, ok := blockMap["text"].(map[string]interface{}); ok {
							textContent, _ := textObj["text"].(string)
							textType, _ := textObj["type"].(string)

							// Force mrkdwn type if specified, otherwise use the specified type
							if textType == "mrkdwn" || textType == "" {
								section.Text = slack.NewTextBlockObject(slack.MarkdownType, textContent, false, false)
							} else {
								section.Text = slack.NewTextBlockObject(slack.PlainTextType, textContent, false, false)
							}
						}
						slackBlock = section
					}
				case "header":
					var header slack.HeaderBlock
					if err := json.Unmarshal(blockJSON, &header); err == nil {
						slackBlock = header
					}
				case "actions":
					var actions slack.ActionBlock
					if err := json.Unmarshal(blockJSON, &actions); err == nil {
						slackBlock = actions
					}
				case "divider":
					slackBlock = slack.NewDividerBlock()
				case "context":
					var context slack.ContextBlock
					if err := json.Unmarshal(blockJSON, &context); err == nil {
						slackBlock = context
					}
					// Build context manually to ensure elements are preserved
					if raw, ok := blockMap["elements"].([]interface{}); ok && len(raw) > 0 {
						elems := make([]slack.MixedElement, 0, len(raw))
						for _, it := range raw {
							m, ok := it.(map[string]interface{})
							if !ok {
								continue
							}
							t, _ := m["type"].(string)
							txt, _ := m["text"].(string)
							if txt == "" {
								continue
							}
							if t == "mrkdwn" {
								elems = append(elems, slack.NewTextBlockObject(slack.MarkdownType, txt, false, false))
							} else {
								elems = append(elems, slack.NewTextBlockObject(slack.PlainTextType, txt, false, false))
							}
						}
						if len(elems) > 0 {
							slackBlock = slack.NewContextBlock("", elems...)
						}
					}
					// Add more block types as needed
				}

				if slackBlock != nil {
					blocks.BlockSet = append(blocks.BlockSet, slackBlock)
				}
			}

			if len(blocks.BlockSet) > 0 {
				// Create fallback text in case blocks fail
				fallbackText := blockMessage.Text
				if fallbackText == "" {
					// If no fallback text provided, use the original text
					fallbackText = text
				}

				// Add the blocks first, then the fallback text
				msgOptions = append(msgOptions, slack.MsgOptionBlocks(blocks.BlockSet...))
				msgOptions = append(msgOptions, slack.MsgOptionText(fallbackText, false))
			} else {
				// Failed to parse blocks, fall back to text
				msgOptions = append(msgOptions, slack.MsgOptionText(text, options.EscapeText))
			}
		} else {
			// Not valid JSON, treat as text
			msgOptions = append(msgOptions, slack.MsgOptionText(text, options.EscapeText))
		}
	case TextFormat:
		// Simple text message with mrkdwn
		msgOptions = append(msgOptions, slack.MsgOptionText(text, options.EscapeText))
	}

	return msgOptions
}

// findCodeBlockBoundaries finds the start and end positions of code blocks in the text
func findCodeBlockBoundaries(text string) []struct{ start, end int } {
	var boundaries []struct{ start, end int }

	// Find all code block markers (```)
	codeBlockRegex := regexp.MustCompile("```")
	matches := codeBlockRegex.FindAllStringIndex(text, -1)

	// Process matches in pairs (start and end of code blocks)
	for i := 0; i < len(matches)-1; i += 2 {
		start := matches[i][0]
		end := matches[i+1][1] // Include the closing ```
		boundaries = append(boundaries, struct{ start, end int }{start, end})
	}

	return boundaries
}

// findSafeBreakPoint finds a safe break point (legacy function for compatibility)
func findSafeBreakPoint(text string, maxChunkSize int) int {
	if len(text) <= maxChunkSize {
		return len(text)
	}

	// Look for the last newline or space
	chunk := text[:maxChunkSize]
	lastNewline := strings.LastIndex(chunk, "\n")
	lastSpace := strings.LastIndex(chunk, " ")

	if lastNewline > int(float64(maxChunkSize)*0.8) {
		return lastNewline + 1
	}
	if lastSpace > int(float64(maxChunkSize)*0.8) {
		return lastSpace + 1
	}

	return maxChunkSize
}

// splitTextPreservingCodeBlocks splits text into chunks while ensuring code blocks are not split
func splitTextPreservingCodeBlocks(text string, maxChunkSize int) []string {
	if len(text) <= maxChunkSize {
		return []string{text}
	}

	var chunks []string
	boundaries := findCodeBlockBoundaries(text)

	// If no code blocks, use the original logic
	if len(boundaries) == 0 {
		return splitTextSimple(text, maxChunkSize)
	}

	// Process text, treating code blocks as atomic units
	pos := 0
	currentChunk := ""

	for pos < len(text) {
		// Check if we're at the start of a code block
		isInCodeBlock := false
		var currentBoundary struct{ start, end int }

		for _, boundary := range boundaries {
			if pos >= boundary.start && pos < boundary.end {
				isInCodeBlock = true
				currentBoundary = boundary
				break
			}
		}

		if isInCodeBlock {
			// Add the entire code block
			codeBlock := text[currentBoundary.start:currentBoundary.end]

			// Check if adding this code block would exceed the chunk size
			if len(currentChunk)+len(codeBlock) > maxChunkSize && len(currentChunk) > 0 {
				// Start a new chunk
				chunks = append(chunks, currentChunk)
				currentChunk = codeBlock
			} else {
				currentChunk += codeBlock
			}

			pos = currentBoundary.end
		} else {
			// Find the next code block or end of text
			nextBoundaryStart := len(text)
			for _, boundary := range boundaries {
				if boundary.start > pos && boundary.start < nextBoundaryStart {
					nextBoundaryStart = boundary.start
				}
			}

			// Take as much text as possible up to the next code block or max chunk size
			availableSpace := maxChunkSize - len(currentChunk)
			endPos := pos + availableSpace

			if endPos >= nextBoundaryStart {
				// We can include up to the next code block
				segment := text[pos:nextBoundaryStart]
				currentChunk += segment
				pos = nextBoundaryStart
			} else {
				// Find a safe break point in the available space
				segment := text[pos:endPos]
				breakPoint := findSafeBreakPointInSegment(segment, availableSpace)

				if breakPoint > 0 {
					currentChunk += text[pos : pos+breakPoint]
					chunks = append(chunks, currentChunk)
					currentChunk = ""
					pos += breakPoint
				} else {
					// No safe break point, take the whole segment
					currentChunk += segment
					pos = endPos

					// If we filled the chunk, start a new one
					if len(currentChunk) >= maxChunkSize {
						chunks = append(chunks, currentChunk)
						currentChunk = ""
					}
				}
			}
		}
	}

	// Add the last chunk if not empty
	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

// splitTextSimple splits text using the original simple logic
func splitTextSimple(text string, maxChunkSize int) []string {
	var chunks []string
	remaining := text

	for len(remaining) > 0 {
		chunkSize := maxChunkSize
		if len(remaining) < chunkSize {
			chunkSize = len(remaining)
		} else {
			// Find a safe break point
			chunkSize = findSafeBreakPoint(remaining, maxChunkSize)
		}

		chunk := remaining[:chunkSize]
		chunks = append(chunks, chunk)
		remaining = remaining[chunkSize:]
	}

	return chunks
}

// findSafeBreakPointInSegment finds a safe break point within a segment
func findSafeBreakPointInSegment(segment string, maxLength int) int {
	if len(segment) <= maxLength {
		return len(segment)
	}

	// Look for the last newline or space
	lastNewline := strings.LastIndex(segment[:maxLength], "\n")
	lastSpace := strings.LastIndex(segment[:maxLength], " ")

	if lastNewline > int(float64(maxLength)*0.8) {
		return lastNewline + 1
	}
	if lastSpace > int(float64(maxLength)*0.8) {
		return lastSpace + 1
	}

	// No good break point found
	return 0
}

// CreateBlockMessage creates a Block Kit message with the given options
func CreateBlockMessage(text string, blockOptions BlockOptions) string {
	blocks := []map[string]interface{}{}

	// Add header if provided
	if blockOptions.HeaderText != "" {
		// Truncate header text if too long (Slack has a 150 char limit for plain_text)
		headerText := blockOptions.HeaderText

		// Add emoji prefix if provided
		if blockOptions.HeaderEmoji != "" {
			headerText = blockOptions.HeaderEmoji + " " + headerText
		}

		if len(headerText) > 150 {
			headerText = headerText[:147] + "..."
		}

		blocks = append(blocks, map[string]interface{}{
			"type": "header",
			"text": map[string]interface{}{
				"type":  "plain_text",
				"text":  headerText,
				"emoji": true, // Enable emoji rendering
			},
		})
	}

	// Add fields if provided
	if len(blockOptions.Fields) > 0 {
		// Slack has a limit of 10 fields per section
		// Split fields into multiple sections if needed
		for i := 0; i < len(blockOptions.Fields); i += 10 {
			end := i + 10
			if end > len(blockOptions.Fields) {
				end = len(blockOptions.Fields)
			}

			fields := []map[string]interface{}{}
			for _, field := range blockOptions.Fields[i:end] {
				// Truncate field text if too long (Slack has a 2000 char limit for text fields)
				fieldValue := field.Value
				if len(fieldValue) > 2000 {
					fieldValue = fieldValue[:1997] + "..."
				}

				fields = append(fields, map[string]interface{}{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*%s*\n%s", field.Title, fieldValue),
				})
			}

			blocks = append(blocks, map[string]interface{}{
				"type":   "section",
				"fields": fields,
			})
		}
	}

	// Add text section if provided
	if text != "" {
		// Slack has a 3000 char limit for text blocks in sections
		// Split long text into multiple section blocks while preserving code blocks
		const maxChunkSize = 2900 // Leave room for markdown formatting

		if len(text) <= maxChunkSize {
			// Text fits in a single block
			blocks = append(blocks, map[string]interface{}{
				"type": "section",
				"text": map[string]interface{}{
					"type": "mrkdwn",
					"text": text,
				},
			})
		} else {
			// Split text into multiple blocks while preserving code blocks
			chunks := splitTextPreservingCodeBlocks(text, maxChunkSize)

			for _, chunk := range chunks {
				blocks = append(blocks, map[string]interface{}{
					"type": "section",
					"text": map[string]interface{}{
						"type": "mrkdwn",
						"text": chunk,
					},
				})
			}
		}
	}

	// Optionally add a divider before the footer
	if blockOptions.DividerBeforeFooter && blockOptions.FooterMrkdwn != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "divider",
		})
	}

	// Add footer if provided
	if blockOptions.FooterMrkdwn != "" {
		footerText := blockOptions.FooterMrkdwn

		// Add emoji prefix if provided
		if blockOptions.FooterIcon != "" {
			footerText = blockOptions.FooterIcon + " " + footerText
		}

		// Add timestamp if requested
		if blockOptions.ShowTimestamp {
			timestamp := time.Now().Format("Jan 2, 2006 3:04 PM MST")
			footerText = footerText + " • " + timestamp
		}

		blocks = append(blocks, map[string]interface{}{
			"type": "context",
			"elements": []map[string]interface{}{
				{
					"type": "mrkdwn",
					"text": footerText,
				},
			},
		})
	}

	// Add actions if provided
	if len(blockOptions.Actions) > 0 {
		// Slack has a limit of 5 elements in an actions block
		actionCount := len(blockOptions.Actions)
		if actionCount > 5 {
			actionCount = 5
		}

		elements := []map[string]interface{}{}
		for i := 0; i < actionCount; i++ {
			action := blockOptions.Actions[i]

			// Truncate button text if too long (Slack has a 75 char limit for button text)
			buttonText := action.Text
			if len(buttonText) > 75 {
				buttonText = buttonText[:72] + "..."
			}

			elements = append(elements, map[string]interface{}{
				"type": "button",
				"text": map[string]interface{}{
					"type": "plain_text",
					"text": buttonText,
				},
				"url": action.URL,
			})
		}

		blocks = append(blocks, map[string]interface{}{
			"type":     "actions",
			"elements": elements,
		})
	}

	// Create the final message
	message := map[string]interface{}{
		"text":   text, // Fallback text
		"blocks": blocks,
	}

	// Convert to JSON
	jsonBytes, err := json.Marshal(message)
	if err != nil {
		return text // Fallback to plain text if JSON marshaling fails
	}

	return string(jsonBytes)
}

// FormatMarkdown formats text using Slack's mrkdwn syntax
func FormatMarkdown(text string) string {
	// NOTE: Removed ConvertQuotedStringsToCode as it was too aggressive
	// and created visual noise with literal backticks in Block Kit messages

	// Replace standard Markdown bold (**text**) with Slack bold (*text*)
	boldPattern := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	text = boldPattern.ReplaceAllString(text, "*$1*")

	// Replace standard Markdown block quotes (>) with Slack block quotes (>)
	quotePattern := regexp.MustCompile(`(?m)^\s*>\s+(.*)$`)
	text = quotePattern.ReplaceAllString(text, "> $1")

	// Convert Markdown headers (# ## ###) to Slack bold text
	headerPattern := regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
	text = headerPattern.ReplaceAllString(text, "*$2*")

	// Convert Markdown links [text](url) to Slack format <url|text>
	markdownLinkPattern := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	text = markdownLinkPattern.ReplaceAllString(text, "<$2|$1>")

	// Convert bare HTTP/HTTPS URLs to Slack clickable links <url>
	// But avoid converting URLs that are already in Slack format
	bareUrlPattern := regexp.MustCompile(`(?:^|[^<])(https?://[^\s<>]+)(?:[^>]|$)`)
	text = bareUrlPattern.ReplaceAllStringFunc(text, func(match string) string {
		// Extract the URL from the match
		urlMatch := regexp.MustCompile(`https?://[^\s<>]+`).FindString(match)
		if urlMatch != "" {
			// Preserve any characters before/after the URL
			prefix := match[:strings.Index(match, urlMatch)]
			suffix := match[strings.Index(match, urlMatch)+len(urlMatch):]
			return prefix + "<" + urlMatch + ">" + suffix
		}
		return match
	})

	return text
}

// ConvertQuotedStringsToCode converts double-quoted strings to inline code blocks
// for better visualization in Slack
func ConvertQuotedStringsToCode(text string) string {
	// Regex to find double-quoted strings
	// This pattern looks for "..." but avoids matching escaped quotes \"...\"
	pattern := regexp.MustCompile(`"([^"\\]*(\\.[^"\\]*)*)"`)

	// Replace each match with a code block
	text = pattern.ReplaceAllString(text, "`$1`")

	// Also handle specific patterns like "yyyy-MM-ddTHH:mm:ssZ" timestamps
	// which are common in Kubernetes and other outputs
	timestampPattern := regexp.MustCompile(`"(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)"`)
	text = timestampPattern.ReplaceAllString(text, "`$1`")

	// Handle quoted namespace names and other identifiers
	identifierPattern := regexp.MustCompile(`"([\w-]+)"`)
	text = identifierPattern.ReplaceAllString(text, "`$1`")

	return text
}

// EscapeMarkdown escapes special characters in Markdown
func EscapeMarkdown(text string) string {
	// Escape &, <, and >
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

// BoldText formats text as bold
func BoldText(text string) string {
	return fmt.Sprintf("*%s*", text)
}

// ItalicText formats text as italic
func ItalicText(text string) string {
	return fmt.Sprintf("_%s_", text)
}

// StrikethroughText formats text with strikethrough
func StrikethroughText(text string) string {
	return fmt.Sprintf("~%s~", text)
}

// CodeText formats text as inline code
func CodeText(text string) string {
	return fmt.Sprintf("`%s`", text)
}

// CodeBlock formats text as a code block
func CodeBlock(text, language string) string {
	if language != "" {
		return fmt.Sprintf("```%s\n%s\n```", language, text)
	}
	return fmt.Sprintf("```\n%s\n```", text)
}

// QuoteText formats text as a quote
func QuoteText(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}

// BulletList creates a bulleted list from items
func BulletList(items []string) string {
	var result strings.Builder
	for _, item := range items {
		result.WriteString("• " + item + "\n")
	}
	return result.String()
}

// NumberedList creates a numbered list from items
func NumberedList(items []string) string {
	var result strings.Builder
	for i, item := range items {
		result.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
	}
	return result.String()
}

// Link creates a Slack link
func Link(url, text string) string {
	if text == "" {
		return url
	}
	return fmt.Sprintf("<%s|%s>", url, text)
}

// UserMention creates a user mention
func UserMention(userID string) string {
	return fmt.Sprintf("<@%s>", userID)
}

// ChannelMention creates a channel mention
func ChannelMention(channelID, channelName string) string {
	if channelName == "" {
		return fmt.Sprintf("<#%s>", channelID)
	}
	return fmt.Sprintf("<#%s|%s>", channelID, channelName)
}

// EmailLink creates an email link
func EmailLink(email, text string) string {
	if text == "" {
		return fmt.Sprintf("<mailto:%s>", email)
	}
	return fmt.Sprintf("<mailto:%s|%s>", email, text)
}

// CreateResponseTemplate creates a beautifully formatted response with optional header
func CreateResponseTemplate(content string, options BlockOptions) string {
	// Apply markdown formatting first
	formattedContent := FormatMarkdown(content)

	// Create the block message with the options
	return CreateBlockMessage(formattedContent, options)
}

// CreateSuccessResponse creates a success-themed response
func CreateSuccessResponse(content, traceURL string) string {
	footer := ""
	if traceURL != "" {
		footer = Link(traceURL, "View Trace")
	}

	return CreateResponseTemplate(content, BlockOptions{
		FooterMrkdwn:        footer,
		FooterIcon:          "✅",
		DividerBeforeFooter: true,
		ShowTimestamp:       true,
	})
}

// CreateErrorResponse creates an error-themed response
func CreateErrorResponse(content, traceURL string) string {
	footer := ""
	if traceURL != "" {
		footer = Link(traceURL, "View Trace")
	}

	return CreateResponseTemplate(content, BlockOptions{
		FooterMrkdwn:        footer,
		FooterIcon:          "❌",
		DividerBeforeFooter: true,
		ShowTimestamp:       true,
	})
}

// CreateInfoResponse creates an info-themed response
func CreateInfoResponse(content, traceURL string) string {
	footer := ""
	if traceURL != "" {
		footer = Link(traceURL, "View Trace")
	}

	return CreateResponseTemplate(content, BlockOptions{
		FooterMrkdwn:        footer,
		FooterIcon:          "ℹ️",
		DividerBeforeFooter: true,
		ShowTimestamp:       true,
	})
}
