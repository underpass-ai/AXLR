package application

import "strings"

// foldedLetters spells the Latin letters a brief commonly carries in ASCII,
// so a slug keeps the word: on 12 October 2026 "Añade en buildinfo…" named
// its plan "a-ade-en-…", the ñ dropped and the word split in two.
var foldedLetters = map[rune]string{
	'á': "a", 'à': "a", 'â': "a", 'ä': "a", 'ã': "a", 'å': "a", 'ā': "a", 'ă': "a", 'ą': "a",
	'Á': "A", 'À': "A", 'Â': "A", 'Ä': "A", 'Ã': "A", 'Å': "A", 'Ā': "A", 'Ă': "A", 'Ą': "A",
	'é': "e", 'è': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ė': "e", 'ę': "e", 'ě': "e",
	'É': "E", 'È': "E", 'Ê': "E", 'Ë': "E", 'Ē': "E", 'Ė': "E", 'Ę': "E", 'Ě': "E",
	'í': "i", 'ì': "i", 'î': "i", 'ï': "i", 'ī': "i", 'į': "i", 'ı': "i",
	'Í': "I", 'Ì': "I", 'Î': "I", 'Ï': "I", 'Ī': "I", 'Į': "I", 'İ': "I",
	'ó': "o", 'ò': "o", 'ô': "o", 'ö': "o", 'õ': "o", 'ø': "o", 'ō': "o", 'ő': "o",
	'Ó': "O", 'Ò': "O", 'Ô': "O", 'Ö': "O", 'Õ': "O", 'Ø': "O", 'Ō': "O", 'Ő': "O",
	'ú': "u", 'ù': "u", 'û': "u", 'ü': "u", 'ū': "u", 'ů': "u", 'ű': "u", 'ų': "u",
	'Ú': "U", 'Ù': "U", 'Û': "U", 'Ü': "U", 'Ū': "U", 'Ů': "U", 'Ű': "U", 'Ų': "U",
	'ý': "y", 'ÿ': "y", 'Ý': "Y", 'Ÿ': "Y",
	'ñ': "n", 'ń': "n", 'ň': "n", 'Ñ': "N", 'Ń': "N", 'Ň': "N",
	'ç': "c", 'ć': "c", 'č': "c", 'Ç': "C", 'Ć': "C", 'Č': "C",
	'ś': "s", 'š': "s", 'ş': "s", 'Ś': "S", 'Š': "S", 'Ş': "S",
	'ź': "z", 'ż': "z", 'ž': "z", 'Ź': "Z", 'Ż': "Z", 'Ž': "Z",
	'ł': "l", 'ľ': "l", 'Ł': "L", 'Ľ': "L",
	'ř': "r", 'Ř': "R", 'ť': "t", 'Ť': "T", 'ď': "d", 'Ď': "D", 'đ': "d", 'Đ': "D", 'ğ': "g", 'Ğ': "G",
	'ß': "ss", 'æ': "ae", 'Æ': "AE", 'œ': "oe", 'Œ': "OE", 'þ': "th", 'Þ': "TH", 'ð': "d", 'Ð': "D",
}

// foldToASCII replaces the accented and ligature Latin letters of text with
// their plain spelling and keeps every other rune as it is.
func foldToASCII(text string) string {
	var out strings.Builder
	for _, r := range text {
		if plain, ok := foldedLetters[r]; ok {
			out.WriteString(plain)
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}
