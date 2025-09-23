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
