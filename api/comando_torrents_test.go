package handler

import (
	"reflect"
	"testing"
	"time"

	"github.com/felipemarinho97/torrent-indexer/schema"
)

func Test_findAudioFromText(t *testing.T) {
	type args struct {
		text string
	}
	tests := []struct {
		name string
		args args
		want []schema.Audio
	}{
		{
			name: "should return audio in portuguese",
			args: args{
				text: "Áudio: Português",
			},
			want: []schema.Audio{
				schema.AudioPortuguese,
			},
		},
		{
			name: "should return audio in portuguese",
			args: args{
				text: "Idioma: Português",
			},
			want: []schema.Audio{
				schema.AudioPortuguese,
			},
		},
		{
			name: "should return audio in portuguese",
			args: args{
				text: "Audio: Português",
			},
			want: []schema.Audio{
				schema.AudioPortuguese,
			},
		},
		{
			name: "should return audio in portuguese",
			args: args{
				text: `
»INFORMAÇÕES«
Título Traduzido: O Cangaceiro do Futuro
Título Original: O Cangaceiro do Futuro
IMDb: 7,1
Gênero:Comédia
Lançamento: 2022
Qualidade: WEB-DL
Áudio: Português
Legenda: S/L
Formato: MKV
Tamanho: 5.77 GB | 9.60 GB
Duração: 30 Min./Ep.
Qualidade de Áudio: 10
Qualidade de Vídeo: 10
Servidor Via: Torrent
				`,
			},
			want: []schema.Audio{
				schema.AudioPortuguese,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findAudioFromText(tt.args.text); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("findAudioFromText() = %v, want %v", got, tt.want)
			}
		})
	}
}

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

func Test_getIMDBLink(t *testing.T) {
	type args struct {
		link string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "should return imdb link",
			args: args{
				link: "https://www.imdb.com/title/tt1234567",
			},
			want:    "https://www.imdb.com/title/tt1234567",
			wantErr: false,
		},
		{
			name: "should return imdb link when end with /",
			args: args{
				link: "https://www.imdb.com/title/tt1234567/",
			},
			want:    "https://www.imdb.com/title/tt1234567/",
			wantErr: false,
		},
		{
			name: "should return imdb link when end with /",
			args: args{
				link: "https://www.imdb.com/title/tt1234567/",
			},
			want:    "https://www.imdb.com/title/tt1234567/",
			wantErr: false,
		},
		{
			name: "should return imdb link when it has a language",
			args: args{
				link: "https://www.imdb.com/pt/title/tt18722864/",
			},
			want: "https://www.imdb.com/pt/title/tt18722864/",
		},
		{
			name: "should return imdb link when it has a language",
			args: args{
				link: "https://www.imdb.com/pt/title/tt34608980/",
			},
			want: "https://www.imdb.com/pt/title/tt34608980/",
		},
		{
			name: "should return error when link is invalid",
			args: args{
				link: "https://www.google.com",
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getIMDBLink(tt.args.link)
			if (err != nil) != tt.wantErr {
				t.Errorf("getIMDBLink() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getIMDBLink() = %v, want %v", got, tt.want)
			}
		})
	}
}
