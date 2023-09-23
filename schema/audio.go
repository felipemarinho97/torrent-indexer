package schema

type Audio string

const (
	AudioPortuguese = "Português"
	AudioEnglish    = "Inglês"
	AudioSpanish    = "Espanhol"
	AudioFrench     = "Francês"
	AudioGerman     = "Alemão"
	AudioItalian    = "Italiano"
	AudioJapanese   = "Japonês"
	AudioKorean     = "Coreano"
	AudioMandarin   = "Mandarim"
	AudioMandarin2  = "Chinês"
	AudioRussian    = "Russo"
	AudioSwedish    = "Sueco"
	AudioUkrainian  = "Ucraniano"
	AudioPolish     = "Polaco"
	AudioPolish2    = "Polonês"
	AudioThai       = "Tailandês"
	AudioTurkish    = "Turco"
)

var AudioList = []Audio{
	AudioPortuguese,
	AudioEnglish,
	AudioSpanish,
	AudioFrench,
	AudioGerman,
	AudioItalian,
	AudioJapanese,
	AudioKorean,
	AudioMandarin,
	AudioMandarin2,
	AudioRussian,
	AudioSwedish,
	AudioUkrainian,
	AudioPolish,
	AudioPolish2,
	AudioThai,
	AudioTurkish,
}

func (a Audio) String() string {
	return a.toISO639_2()
}

func GetAudioFromString(s string) *Audio {
	for _, a := range AudioList {
		if string(a) == s {
			return &a
		}
	}
	return nil
}

func (a Audio) toISO639_2() string {
	switch a {
	case AudioPortuguese:
		return "por"
	case AudioEnglish:
		return "eng"
	case AudioSpanish:
		return "spa"
	case AudioFrench:
		return "fra"
	case AudioGerman:
		return "deu"
	case AudioItalian:
		return "ita"
	case AudioJapanese:
		return "jpn"
	case AudioKorean:
		return "kor"
	case AudioMandarin:
		return "chi"
	case AudioMandarin2:
		return "chi"
	case AudioRussian:
		return "rus"
	case AudioSwedish:
		return "swe"
	case AudioUkrainian:
		return "ukr"
	case AudioPolish:
		return "pol"
	case AudioPolish2:
		return "pol"
	case AudioThai:
		return "tha"
	case AudioTurkish:
		return "tur"
	default:
		return ""
	}
}
