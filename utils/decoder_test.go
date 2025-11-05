package utils_test

import (
	"testing"

	"github.com/felipemarinho97/torrent-indexer/utils"
)

func TestDecodeAdLink(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		encodedStr string
		want       string
		wantErr    bool
	}{
		{
			name:       "Valid encoded string",
			encodedStr: "jVzYmJjZxYjYwMDZiVjZ2UTMmJGM3EmZ4E2M2cDZ0UGN4UmN5EWOlpDapRnY64mc11Dd49jO0VmbnFWb",
			want:       "magnet:?xt=urn:btih:e9a96e84e4d763a8fa70bf156f5bd30b61f2fc5c",
			wantErr:    false,
		},
		{
			name:       "Invalid encoded string",
			encodedStr: "invalid_encoded_string",
			want:       "",
			wantErr:    true,
		},
		{
			name:       "Empty string",
			encodedStr: "",
			want:       "",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := utils.DecodeAdLink(tt.encodedStr)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("DecodeAdLink() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("DecodeAdLink() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("DecodeAdLink() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBase64Decode(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "Valid base64 string",
			input:   "bWFnbmV0Oj94dD11cm46YnRpaDpoMWIxOWYxNmM0MmMyNWMxNGZhNmNhNzY2NGNhNzZlN2Y2NDZhM2Q2NGY=",
			want:    "magnet:?xt=urn:btih:h1b19f16c42c25c14fa6ca7664ca76e7f646a3d64f",
			wantErr: false,
		},
		{
			name:    "Invalid base64 string",
			input:   "invalid_base64_string",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Empty string",
			input:   "",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := utils.Base64Decode(tt.input)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Base64Decode() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Base64Decode() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("Base64Decode() = %v, want %v", got, tt.want)
			}
		})
	}
}
