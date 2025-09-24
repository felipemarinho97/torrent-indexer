package handler

import (
	"reflect"
	"testing"
	"time"

	"github.com/felipemarinho97/torrent-indexer/schema"
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
				dateStr: "https://www.starckfilmes.fans/catalog/jogos-de-seducao-2025-18-07-2025/",
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
			name: "should return audio in portuguese - comando_torrents",
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
		{
			name: "should return audio in portuguese - rede torrent",
			args: args{
				text: `
Filme Bicho de Sete Cabeças Torrent
Título Original: Bicho de Sete Cabeças
Lançamento: 2001
Gêneros: Drama / Nacional
Idioma: Português
Qualidade: 720p / BluRay
Duração: 1h 14 Minutos
Formato: Mp4
Vídeo: 10 e Áudio: 10
Legendas: Português
Nota do Imdb: 7.7
Tamanho: 1.26 GB
				`,
			},
			want: []schema.Audio{
				schema.AudioPortuguese,
			},
		},
		{
			name: "should return audio in portuguese - rede torrent 2",
			args: args{
				text: `
Filme Branca de Neve e o Caçador Torrent / Assistir Online
Título Original: Snow White and the Huntsman
Lançamento: 2012
Gêneros: Ação / Aventura / Fantasia
Idioma: Português / Inglês
Duração: 126 Minutos
Formato: Mkv / Mp4
Vídeo: 10 e Áudio: 10
Legendas: Sim
Tamanho: 2.69 GB / 1.95 GB / 1.0 GB
				`,
			},
			want: []schema.Audio{
				schema.AudioPortuguese,
				schema.AudioEnglish,
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

func Test_getAudioFromTitle(t *testing.T) {
	tests := []struct {
		name             string
		releaseTitle     string
		audioFromContent []schema.Audio
		want             []schema.Audio
	}{
		{
			name:             "should return portuguese audio when title contains dual",
			releaseTitle:     "A Bailarina (2017) Dual Áudio BluRay 720p | 1080p – Torrent Download",
			audioFromContent: []schema.Audio{schema.AudioFrench},
			want:             []schema.Audio{schema.AudioPortuguese, schema.AudioFrench},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAudioFromTitle(tt.releaseTitle, tt.audioFromContent)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getAudioFromTitle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_appendAudioISO639_2Code(t *testing.T) {
	tests := []struct {
		name  string
		title string
		a     []schema.Audio
		want  string
	}{
		{
			name:  "should append audio in title",
			title: "Movie Title",
			a: []schema.Audio{
				schema.AudioPortuguese,
				schema.AudioEnglish,
			},
			want: "Movie Title (brazilian, eng)",
		},
		{
			name:  "should append audio in title and deduplicate",
			title: "Movie Title",
			a: []schema.Audio{
				schema.AudioPortuguese,
				schema.AudioPortuguese2,
				schema.AudioEnglish,
				schema.AudioEnglish2,
			},
			want: "Movie Title (brazilian, eng)",
		},
		{
			name:  "should not append empty audio",
			title: "Movie Title",
			a:     []schema.Audio{},
			want:  "Movie Title",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendAudioISO639_2Code(tt.title, tt.a)

			if got != tt.want {
				t.Errorf("appendAudioISO639_2Code() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_findYearFromText(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		title string
		want  string
	}{
		{
			name:  "should find year from text",
			text:  "Some Movie Title\nLançamento: 2020\nTipo: Filme\nDescription: The movie was released in 2020.",
			title: "Some Movie Title",
			want:  "2020",
		},
		{
			name:  "should find year from text with patten YYYY-MM-DD",
			text:  "Some Movie Title\nLançamento: 2020-06-46\nTipo: Filme\nDescription: The movie is so good.",
			title: "Some Movie Title",
			want:  "2020",
		},
		{
			name:  "should find year from text with patten DD/MM/YYYY",
			text:  "Some Movie Title\nLançamento: 12/06/2020\nTipo: Filme\nDescription: The movie is so good.",
			title: "Some Movie Title",
			want:  "2020",
		},
		{
			name:  "should find year from title",
			text:  "Some Movie Title without year info",
			title: "Some Movie Title (2021)",
			want:  "2021",
		},
		{
			name:  "should return empty string when year is not found",
			text:  "No year information here.",
			title: "Another Movie Title",
			want:  "",
		},
		{
			name:  "should return empty string when year is not found",
			text:  "Some Movie Title\nLançamento: Vários anos.\nTipo: Filme\nDescription: The movie was released in 2020.",
			title: "Another Movie Title",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findYearFromText(tt.text, tt.title)
			if got != tt.want {
				t.Errorf("findYearFromText() = %v, want %v", got, tt.want)
			}
		})
	}
}
