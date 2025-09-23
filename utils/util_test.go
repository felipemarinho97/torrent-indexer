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

func TestDecodeAdLink(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		// Valid decoding cases
		{
			name:    "Simple text decoding",
			input:   "b2xsZWg=", // "hello" -> base64 -> reverse
			want:    "hello",
			wantErr: false,
		},
		{
			name:    "Magnet link decoding",
			input:   "PTswMzIzNGU4YmE1YTYwZjk4ODJkNzI5YTQwNzc4NzA0Zjc2OTRmOGU6aHNhaD0mbnI9JnRuPXRlc3Q6dGVuZ2Ft", // base64 encoded reversed magnet link
			want:    "magnet:?xt=urn:btih:e8f49670f4870740a927d2889f06a5ab8e43230&dn=test&tr=",
			wantErr: false,
		},
		{
			name:    "HTML escaped content",
			input:   "Jm51bTsmYW1wOyZ0bDsmbG90O29sbGVoJnRnOw==", // "&lt;hello&gt;&amp;&num;" -> base64 -> reverse
			want:    "<hello>&",
			wantErr: false,
		},
		{
			name:    "URL with parameters",
			input:   "PTEmdz0xJnE9dGVzdCE/bW9jLmVscG1heGUud3d3Oi8vcHR0aA==", // "http://www.example.com?test=q&w=1!" -> base64 -> reverse
			want:    "http://www.example.com?test=q&w=1!",
			wantErr: false,
		},
		{
			name:    "Empty string",
			input:   "",
			want:    "",
			wantErr: false,
		},
		{
			name:    "Unicode content",
			input:   "8J+YgPCfmIA=", // "😀😀" -> base64 -> reverse
			want:    "😀😀",
			wantErr: false,
		},
		// Invalid base64 cases
		{
			name:    "Invalid base64 - odd length",
			input:   "SGVsbG8gd29ybGQ", // Missing padding, invalid base64
			want:    "",
			wantErr: true,
		},
		{
			name:    "Invalid base64 characters",
			input:   "Hello@#$%^&*()",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Invalid base64 - special characters",
			input:   "SGVsbG8_d29ybGQ=", // Contains underscore which is not valid base64
			want:    "",
			wantErr: true,
		},
		{
			name:    "Partially valid base64",
			input:   "SGVsbG8gV29ybGQ=invalid",
			want:    "",
			wantErr: true,
		},
		// Edge cases
		{
			name:    "Single character",
			input:   "YQ==", // "a" -> base64 -> reverse
			want:    "a",
			wantErr: false,
		},
		{
			name:    "Multiple HTML entities",
			input:   "Jm51bTsmcXVvdDsmYW1wOyZ0bDsmbG90O3RzZXQmdDsmbmc7", // "&lt;test&gt;&amp;&quot;&num;" -> base64 -> reverse
			want:    "<test>&\"",
			wantErr: false,
		},
		{
			name:    "Long content",
			input:   createLongEncodedString(),
			want:    "This is a very long string that should be properly decoded even when it contains many characters and potentially complex encoding scenarios.",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := utils.DecodeAdLink(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeAdLink() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DecodeAdLink() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Helper function to create a long encoded string for testing
func createLongEncodedString() string {
	// This represents the base64 encoded and reversed version of a long string
	// Original: "This is a very long string that should be properly decoded even when it contains many characters and potentially complex encoding scenarios."
	return "LnNvaXJhbmVjcyBnbmlkb2NuZSB4ZWxwbW9jIHlsbGFpdG5ldG9wIGRuYSBzcmV0Y2FyYWhjIHluYW0gc25pYXRub2MgdGkgbmVodyBuZXZlIGRlZG9jZWQgeWxycGVvcCBlYiBkbHVvaHMgdGFodCBnbmlydHMgZ25vbCB5cmV2IGEgc2kgc2loVA=="
}
