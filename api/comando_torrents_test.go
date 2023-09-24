package handler

import (
	"reflect"
	"testing"

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
