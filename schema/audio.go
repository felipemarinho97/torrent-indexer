package schema

import "strings"

type Audio string

const (
	AudioPortuguese  = "Português"
	AudioPortuguese2 = "Portugues"
	AudioPortuguese3 = "PT-BR"
	AudioPortuguese4 = "Dublado"
	AudioEnglish     = "Inglês"
	AudioEnglish2    = "Ingles"
	AudioSpanish     = "Espanhol"
	AudioFrench      = "Francês"
	AudioFrench2     = "Frances"
	AudioGerman      = "Alemão"
	AudioGerman2     = "Alemao"
	AudioItalian     = "Italiano"
	AudioJapanese    = "Japonês"
	AudioJapanese2   = "Japones"
	AudioKorean      = "Coreano"
	AudioMandarin    = "Mandarim"
	AudioMandarin2   = "Chinês"
	AudioMandarin3   = "Chines"
	AudioRussian     = "Russo"
	AudioSwedish     = "Sueco"
	AudioSwedish2    = "Suéco"
	AudioSerbian     = "Sérvio"
	AudioSerbian2    = "Servio"
	AudioUkrainian   = "Ucraniano"
	AudioPolish      = "Polaco"
	AudioPolish2     = "Polonês"
	AudioPolish3     = "Polones"
	AudioThai        = "Tailandês"
	AudioThai2       = "Tailandes"
	AudioTurkish     = "Turco"
	AudioHindi       = "Hindi"
	AudioHungarian   = "Húngaro"
	AudioHungarian2  = "Hungaro"
	AudioFarsi       = "Persa"
	AudioFarsi2      = "Farsi"
	AudioFarsi3      = "Iraniano"
	AudioMalay       = "Malaio"
	AudioDutch       = "Holandês"
	AudioDutch2      = "Holandes"
	AudioFinnish     = "Finlandês"
	AudioFinnish2    = "Finlandes"
	AudioDanish      = "Dinamarquês"
	AudioDanish2     = "Dinamarques"
	AudioNorwegian   = "Norueguês"
	AudioNorwegian2  = "Noruegues"
	AudioIcelandic   = "Islandês"
	AudioIcelandic2  = "Islandes"
	AudioGreek       = "Grego"
	AudioArabic      = "Árabe"
	AudioArabic2     = "Arabe"
	AudioHebrew      = "Hebraico"
	AudioVietnamese  = "Vietnamita"
	AudioIndonesian  = "Indonésio"
	AudioIndonesian2 = "Indonesio"
	AudioFilipino    = "Filipino"
	AudioBengali     = "Bengali"
	AudioTamil       = "Tamil"
	AudioTelugu      = "Telugu"
	AudioGujarati    = "Gujarati"
	AudioMarathi     = "Marathi"
)

var AudioList = []Audio{
	AudioPortuguese,
	AudioPortuguese2,
	AudioPortuguese3,
	AudioPortuguese4,
	AudioEnglish,
	AudioEnglish2,
	AudioSpanish,
	AudioFrench,
	AudioFrench2,
	AudioGerman,
	AudioGerman2,
	AudioItalian,
	AudioJapanese,
	AudioJapanese2,
	AudioKorean,
	AudioMandarin,
	AudioMandarin2,
	AudioMandarin3,
	AudioRussian,
	AudioSwedish,
	AudioSwedish2,
	AudioSerbian,
	AudioSerbian2,
	AudioUkrainian,
	AudioPolish,
	AudioPolish2,
	AudioPolish3,
	AudioThai,
	AudioThai2,
	AudioTurkish,
	AudioHindi,
	AudioHungarian,
	AudioHungarian2,
	AudioFarsi,
	AudioFarsi2,
	AudioFarsi3,
	AudioMalay,
	AudioDutch,
	AudioDutch2,
	AudioFinnish,
	AudioFinnish2,
	AudioDanish,
	AudioDanish2,
	AudioNorwegian,
	AudioNorwegian2,
	AudioIcelandic,
	AudioIcelandic2,
	AudioGreek,
	AudioArabic,
	AudioArabic2,
	AudioHebrew,
	AudioVietnamese,
	AudioIndonesian,
	AudioIndonesian2,
	AudioFilipino,
	AudioBengali,
	AudioTamil,
	AudioTelugu,
	AudioGujarati,
	AudioMarathi,
}

func (a Audio) String() string {
	return a.toTag()
}

func GetAudioFromString(s string) *Audio {
	for _, a := range AudioList {
		if strings.EqualFold(string(a), s) {
			return &a
		}
	}
	return nil
}

func (a Audio) toTag() string {
	switch a {
	case AudioPortuguese:
		return "brazilian"
	case AudioPortuguese2:
		return "brazilian"
	case AudioPortuguese3:
		return "brazilian"
	case AudioPortuguese4:
		return "brazilian"
	case AudioEnglish:
		return "eng"
	case AudioEnglish2:
		return "eng"
	case AudioSpanish:
		return "spa"
	case AudioFrench:
		return "fra"
	case AudioFrench2:
		return "fra"
	case AudioGerman:
		return "deu"
	case AudioGerman2:
		return "deu"
	case AudioItalian:
		return "ita"
	case AudioJapanese:
		return "jpn"
	case AudioJapanese2:
		return "jpn"
	case AudioKorean:
		return "kor"
	case AudioMandarin:
		return "chi"
	case AudioMandarin2:
		return "chi"
	case AudioMandarin3:
		return "chi"
	case AudioRussian:
		return "rus"
	case AudioSwedish:
		return "swe"
	case AudioSwedish2:
		return "swe"
	case AudioSerbian:
		return "srb"
	case AudioSerbian2:
		return "srb"
	case AudioUkrainian:
		return "ukr"
	case AudioPolish:
		return "pol"
	case AudioPolish2:
		return "pol"
	case AudioPolish3:
		return "pol"
	case AudioThai:
		return "tha"
	case AudioThai2:
		return "tha"
	case AudioTurkish:
		return "tur"
	case AudioHindi:
		return "hin"
	case AudioHungarian:
		return "hun"
	case AudioHungarian2:
		return "hun"
	case AudioFarsi:
		return "fas"
	case AudioFarsi2:
		return "fas"
	case AudioFarsi3:
		return "fas"
	case AudioMalay:
		return "msa"
	case AudioDutch:
		return "nld"
	case AudioDutch2:
		return "nld"
	case AudioFinnish:
		return "fin"
	case AudioFinnish2:
		return "fin"
	case AudioDanish:
		return "dan"
	case AudioDanish2:
		return "dan"
	case AudioNorwegian:
		return "nor"
	case AudioNorwegian2:
		return "nor"
	case AudioIcelandic:
		return "isl"
	case AudioIcelandic2:
		return "isl"
	case AudioGreek:
		return "ell"
	case AudioArabic:
		return "ara"
	case AudioArabic2:
		return "ara"
	case AudioHebrew:
		return "heb"
	case AudioVietnamese:
		return "vie"
	case AudioIndonesian:
		return "ind"
	case AudioIndonesian2:
		return "ind"
	case AudioFilipino:
		return "fil"
	case AudioBengali:
		return "ben"
	case AudioTamil:
		return "tam"
	case AudioTelugu:
		return "tel"
	case AudioGujarati:
		return "guj"
	case AudioMarathi:
		return "mar"
	default:
		return ""
	}
}
