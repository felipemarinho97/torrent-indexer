package handler

import (
	"reflect"
	"testing"
	"time"
)

func Test_parseLocalizedDate(t *testing.T) {
	type args struct {
		datePublished string
	}
	tests := []struct {
		name    string
		args    args
		want    time.Time
		wantErr bool
	}{
		{
			name: "should return date",
			args: args{
				datePublished: "12 de outubro de 2022",
			},
			want:    time.Date(2022, 10, 12, 0, 0, 0, 0, time.UTC),
			wantErr: false,
		},
		{
			name: "should return date single digit",
			args: args{
				datePublished: "1 de outubro de 2022",
			},
			want:    time.Date(2022, 10, 1, 0, 0, 0, 0, time.UTC),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLocalizedDate(tt.args.datePublished)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseDate() = %v, want %v", got, tt.want)
			}
		})
	}
}
