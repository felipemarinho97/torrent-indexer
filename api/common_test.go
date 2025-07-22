package handler

import (
	"reflect"
	"testing"
	"time"
)

func Test_getPublishedDateFromRawString(t *testing.T) {
	type args struct {
		dateStr string
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "should parse date in format 2025-01-01",
			args: args{
				dateStr: "2025-01-01",
			},
			want: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "should parse date in format 01-01-2025",
			args: args{
				dateStr: "01-01-2025",
			},
			want: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "should parse date in format 01/01/2025",
			args: args{
				dateStr: "01/01/2025",
			},
			want: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "should parse date from starck-filmes link",
			args: args{
				dateStr: "https://www.starckfilmes.online/catalog/jogos-de-seducao-2025-18-07-2025/",
			},
			want: time.Date(2025, 7, 18, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getPublishedDateFromRawString(tt.args.dateStr); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getPublishedDateFromRawString() = %v, want %v", got, tt.want)
			}
		})
	}
}
