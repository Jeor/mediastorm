package language

import (
	"strings"
)

// ISO 639-1 aliases used by provider results and container track metadata.
var shortCodes = map[string]string{
	"en": "eng", "es": "spa", "fr": "fra", "de": "deu", "it": "ita",
	"pt": "por", "ru": "rus", "ja": "jpn", "zh": "zho", "ko": "kor",
	"nl": "nld", "pl": "pol", "sv": "swe", "cs": "ces", "hu": "hun",
	"tr": "tur", "ar": "ara", "hi": "hin", "th": "tha", "vi": "vie",
	"id": "ind", "da": "dan", "no": "nor", "fi": "fin", "el": "ell",
	"he": "heb", "uk": "ukr", "ro": "ron", "bg": "bul", "hr": "hrv",
	"sl": "slv", "sr": "srp",
}

// codeToEmojis maps ISO 639-2/3 codes to their corresponding emoji flags
var codeToEmojis = map[string][]string{
	"eng": {"🇬🇧", "🇺🇸", "🇦🇺", "🇨🇦", "🇳🇿", "🇮🇪"},
	"spa": {"🇪🇸", "🇲🇽", "🇦🇷", "🇨🇴", "🇨🇱"},
	"fra": {"🇫🇷", "🇨🇦", "🇧🇪", "🇨🇭"},
	"deu": {"🇩🇪", "🇦🇹", "🇨🇭"},
	"ita": {"🇮🇹", "🇨🇭"},
	"por": {"🇵🇹", "🇧🇷"},
	"rus": {"🇷🇺"},
	"jpn": {"🇯🇵"},
	"zho": {"🇨🇳", "🇹🇼", "🇭🇰"},
	"chi": {"🇨🇳", "🇹🇼", "🇭🇰"}, // Alternative code for Chinese
	"kor": {"🇰🇷"},
	"nld": {"🇳🇱", "🇧🇪"},
	"dut": {"🇳🇱", "🇧🇪"}, // Alternative code for Dutch
	"pol": {"🇵🇱"},
	"swe": {"🇸🇪"},
	"ces": {"🇨🇿"},
	"cze": {"🇨🇿"}, // Alternative code for Czech
	"hun": {"🇭🇺"},
	"tur": {"🇹🇷"},
	"ara": {"🇸🇦", "🇪🇬", "🇦🇪"},
	"hin": {"🇮🇳"},
	"tha": {"🇹🇭"},
	"vie": {"🇻🇳"},
	"ind": {"🇮🇩"},
	"dan": {"🇩🇰"},
	"nor": {"🇳🇴"},
	"fin": {"🇫🇮"},
	"gre": {"🇬🇷"},
	"ell": {"🇬🇷"}, // Alternative code for Greek
	"heb": {"🇮🇱"},
	"ukr": {"🇺🇦"},
	"ron": {"🇷🇴"},
	"rum": {"🇷🇴"}, // Alternative code for Romanian
	"bul": {"🇧🇬"},
	"hrv": {"🇭🇷"},
	"slv": {"🇸🇮"},
	"srp": {"🇷🇸"},
}

// emojiToCode maps emoji flags back to ISO codes
var emojiToCode = map[string]string{
	"🇬🇧": "eng", "🇺🇸": "eng", "🇦🇺": "eng", "🇨🇦": "eng", "🇳🇿": "eng", "🇮🇪": "eng",
	"🇪🇸": "spa", "🇲🇽": "spa", "🇦🇷": "spa", "🇨🇴": "spa", "🇨🇱": "spa",
	"🇫🇷": "fra",
	"🇩🇪": "deu", "🇦🇹": "deu",
	"🇮🇹": "ita",
	"🇵🇹": "por", "🇧🇷": "por",
	"🇷🇺": "rus",
	"🇯🇵": "jpn",
	"🇨🇳": "zho", "🇹🇼": "zho", "🇭🇰": "zho",
	"🇰🇷": "kor",
	"🇳🇱": "nld",
	"🇧🇪": "nld", // Could be French or Dutch, defaulting to Dutch
	"🇨🇭": "deu", // Could be German, French, or Italian, defaulting to German
	"🇵🇱": "pol",
	"🇸🇪": "swe",
	"🇨🇿": "ces",
	"🇭🇺": "hun",
	"🇹🇷": "tur",
	"🇸🇦": "ara", "🇪🇬": "ara", "🇦🇪": "ara",
	"🇮🇳": "hin",
	"🇹🇭": "tha",
	"🇻🇳": "vie",
	"🇮🇩": "ind",
	"🇩🇰": "dan",
	"🇳🇴": "nor",
	"🇫🇮": "fin",
	"🇬🇷": "gre",
	"🇮🇱": "heb",
	"🇺🇦": "ukr",
	"🇷🇴": "ron",
	"🇧🇬": "bul",
	"🇭🇷": "hrv",
	"🇸🇮": "slv",
	"🇷🇸": "srp",
}

// nameToCode maps language names to ISO codes (case-insensitive lookup)
var nameToCode = map[string]string{
	"english":    "eng",
	"spanish":    "spa",
	"french":     "fra",
	"german":     "deu",
	"italian":    "ita",
	"portuguese": "por",
	"russian":    "rus",
	"japanese":   "jpn",
	"chinese":    "zho",
	"mandarin":   "zho",
	"cantonese":  "zho",
	"korean":     "kor",
	"dutch":      "nld",
	"polish":     "pol",
	"polski":     "pol",
	"swedish":    "swe",
	"czech":      "ces",
	"hungarian":  "hun",
	"turkish":    "tur",
	"arabic":     "ara",
	"hindi":      "hin",
	"thai":       "tha",
	"vietnamese": "vie",
	"indonesian": "ind",
	"danish":     "dan",
	"norwegian":  "nor",
	"finnish":    "fin",
	"greek":      "gre",
	"hebrew":     "heb",
	"ukrainian":  "ukr",
	"romanian":   "ron",
	"bulgarian":  "bul",
	"croatian":   "hrv",
	"slovenian":  "slv",
	"serbian":    "srp",
	"multi":      "mul", // Multi-language
}

// NormalizeToCode converts any language format (emoji flag, name, or ISO code) to ISO code.
// Returns empty string if the language cannot be identified.
func NormalizeToCode(lang string) string {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return ""
	}

	// Check if it's already an ISO code (3 letters)
	lowerLang := strings.ToLower(lang)
	// Region-tagged codes identify the same audio/subtitle language.
	if i := strings.IndexAny(lowerLang, "-_"); i > 0 {
		lowerLang = lowerLang[:i]
	}
	if code, ok := shortCodes[lowerLang]; ok {
		return code
	}
	switch lowerLang {
	case "fre":
		return "fra"
	case "ger":
		return "deu"
	}
	if len(lowerLang) == 3 {
		// Verify it's a known code by checking if it's in codeToEmojis
		if _, ok := codeToEmojis[lowerLang]; ok {
			return lowerLang
		}
	}

	// Check if it's an emoji flag
	if code, ok := emojiToCode[lang]; ok {
		return code
	}

	// Check if it's a language name
	if code, ok := nameToCode[lowerLang]; ok {
		return code
	}

	return ""
}

// HasPreferredLanguage checks if any of the result languages match the preferred language code.
// resultLanguages is a comma-separated string of languages (can be emoji flags, names, or codes).
// preferredLangCode is the ISO 639-2/3 code of the preferred language (e.g., "eng", "spa").
func HasPreferredLanguage(resultLanguages, preferredLangCode string) bool {
	if resultLanguages == "" || preferredLangCode == "" {
		return false
	}

	preferredLangCode = strings.ToLower(strings.TrimSpace(preferredLangCode))
	if normalized := NormalizeToCode(preferredLangCode); normalized != "" {
		preferredLangCode = normalized
	}
	if preferredLangCode == "" {
		return false
	}

	// Get all valid codes for the preferred language (handles alternative codes)
	// For example, "chi" and "zho" both mean Chinese
	validCodes := getEquivalentCodes(preferredLangCode)

	// Split result languages and check each one
	for _, lang := range strings.Split(resultLanguages, ",") {
		lang = strings.TrimSpace(lang)
		if lang == "" {
			continue
		}

		normalizedCode := NormalizeToCode(lang)
		if normalizedCode == "" {
			continue
		}

		// Check if the normalized code matches any of the valid codes
		for _, validCode := range validCodes {
			if normalizedCode == validCode {
				return true
			}
		}
	}

	return false
}

// getEquivalentCodes returns all ISO codes that represent the same language.
// For example, "zho" and "chi" both represent Chinese.
func getEquivalentCodes(code string) []string {
	equivalents := map[string][]string{
		"zho": {"zho", "chi"},
		"chi": {"zho", "chi"},
		"nld": {"nld", "dut"},
		"dut": {"nld", "dut"},
		"ces": {"ces", "cze"},
		"cze": {"ces", "cze"},
		"gre": {"gre", "ell"},
		"ell": {"gre", "ell"},
		"ron": {"ron", "rum"},
		"rum": {"ron", "rum"},
	}

	if codes, ok := equivalents[code]; ok {
		return codes
	}
	return []string{code}
}
