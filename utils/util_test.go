package utils_test

import (
	"testing"

	"github.com/felipemarinho97/torrent-indexer/utils"
)

func TestIsValidHTML(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		input string
		want  bool
	}{
		// Valid HTML cases
		{
			name:  "Valid HTML with DOCTYPE",
			input: "<!DOCTYPE html><html><head><title>Test</title></head><body><h1>Hello</h1></body></html>",
			want:  true,
		},
		{
			name:  "Valid HTML with html tags",
			input: "<html><head><title>Test</title></head><body><h1>Hello</h1></body></html>",
			want:  true,
		},
		{
			name:  "Valid HTML with body tags only",
			input: "<body><h1>Hello World</h1><p>This is a test.</p></body>",
			want:  true,
		},
		{
			name:  "Valid HTML with DOCTYPE case insensitive",
			input: "<!doctype HTML><html><body>Test</body></html>",
			want:  true,
		},
		{
			name:  "Valid HTML with attributes in html tag",
			input: "<html lang=\"en\" class=\"no-js\"><body>Content</body></html>",
			want:  true,
		},
		{
			name:  "Valid HTML with attributes in body tag",
			input: "<body class=\"main\" id=\"content\"><div>Hello</div></body>",
			want:  true,
		},
		{
			name: "Valid HTML with multiline and whitespace",
			input: `
			<!DOCTYPE html>
			<html>
				<head>
					<title>Test</title>
				</head>
				<body>
					<h1>Hello</h1>
				</body>
			</html>`,
			want: true,
		},
		// Invalid HTML cases
		{
			name:  "Empty string",
			input: "",
			want:  false,
		},
		{
			name:  "Plain text",
			input: "This is just plain text without any HTML",
			want:  false,
		},
		{
			name:  "Script tag alone should not count as valid HTML",
			input: "<script>alert('hello');</script>",
			want:  false,
		},
		{
			name:  "Multiple script tags should not count as valid HTML",
			input: "<script src=\"app.js\"></script><script>console.log('test');</script>",
			want:  false,
		},
		{
			name:  "Div tag alone should not count as valid HTML",
			input: "<div>Some content</div>",
			want:  false,
		},
		{
			name:  "Multiple HTML elements without html/body/doctype",
			input: "<h1>Title</h1><p>Paragraph</p><div>Content</div>",
			want:  false,
		},
		{
			name:  "Style tag alone should not count as valid HTML",
			input: "<style>body { color: red; }</style>",
			want:  false,
		},
		{
			name:  "Meta tag alone should not count as valid HTML",
			input: "<meta charset=\"utf-8\">",
			want:  false,
		},
		{
			name:  "Link tag alone should not count as valid HTML",
			input: "<link rel=\"stylesheet\" href=\"style.css\">",
			want:  false,
		},
		{
			name:  "Mixed content without proper HTML structure",
			input: "Some text <script>code</script> more text <div>content</div>",
			want:  false,
		},
		{
			name:  "Only opening html tag without closing",
			input: "<html><head><title>Test</title>",
			want:  false,
		},
		{
			name:  "Only opening body tag without closing",
			input: "<body><h1>Hello",
			want:  false,
		},
		{
			name:  "Malformed DOCTYPE",
			input: "<!DOCT html><div>content</div>",
			want:  false,
		},
		{
			name:  "JSON content",
			input: `{"key": "value", "array": [1, 2, 3]}`,
			want:  false,
		},
		{
			name:  "XML content",
			input: `<?xml version="1.0"?><root><item>value</item></root>`,
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := utils.IsValidHTML(tt.input)
			if got != tt.want {
				t.Errorf("IsValidHTML() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		sizeStr string
		want    int64
	}{
		{
			name:    "Bytes without space",
			sizeStr: "512B",
			want:    512,
		},
		{
			name:    "Kilobytes with space",
			sizeStr: "1.5 KB",
			want:    1536,
		},
		{
			name:    "Megabytes with comma",
			sizeStr: "2,75 MB",
			want:    2883584,
		},
		{
			name:    "Gigabytes without space",
			sizeStr: "3GB",
			want:    3221225472,
		},
		{
			name:    "Terabytes with space",
			sizeStr: "0.5 TB",
			want:    549755813888,
		},
		{
			name:    "Invalid format",
			sizeStr: "100 XB",
			want:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := utils.ParseSize(tt.sizeStr)

			if got != tt.want {
				t.Errorf("ParseSize() = %v, want %v", got, tt.want)
			}
		})
	}
}
