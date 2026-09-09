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

func TestDecodeStarckDataU(t *testing.T) {
	tests := []struct {
		name    string
		dataU   string
		want    string
		wantErr bool
	}{
		{
			name:    "Valid data-u magnet link",
			dataU:   "mb6ab5g78n4de63tb0:7d?2bx12t69=1bu55r94na2:86beft8fi50h0c:1c",
			want:    "magnet:?xt=urn:btih:bb746b7216159a8e8501658d30db29b5426ff0cc",
			wantErr: false,
		},
		{
			name:    "Invalid data-u string",
			dataU:   "invalid_data_u_string",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Empty data-u",
			dataU:   "",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := utils.DecodeStarckDataU(tt.dataU)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("DecodeStarckDataU() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("DecodeStarckDataU() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("DecodeStarckDataU() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParse1337xDate(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string // expected "2006-01-02"
		wantErr bool
	}{
		{
			name: "standard format with ordinal and short year",
			raw:  "May. 11th '20",
			want: "2020-05-11",
		},
		{
			name: "1st ordinal",
			raw:  "Jan. 1st '99",
			want: "1999-01-01",
		},
		{
			name: "2nd ordinal",
			raw:  "Feb. 2nd '05",
			want: "2005-02-02",
		},
		{
			name: "3rd ordinal",
			raw:  "Mar. 3rd '30",
			want: "2030-03-03",
		},
		{
			name: "no dot on month",
			raw:  "Dec 25th '19",
			want: "2019-12-25",
		},
		{
			name:    "empty string",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "unparseable garbage",
			raw:     "yesterday",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := utils.Parse1337xDate(tt.raw)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("Parse1337xDate(%q) unexpected error: %v", tt.raw, err)
				}
				return
			}
			if tt.wantErr {
				t.Fatalf("Parse1337xDate(%q) succeeded unexpectedly, got %q", tt.raw, got)
			}
			if got != tt.want {
				t.Errorf("Parse1337xDate(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
