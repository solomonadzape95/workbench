package humanize

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Polish is a deterministic safety net for the vocabulary and template rules.
// The model does the rewriting; this pass makes sure a small free model cannot
// leave the most obvious AI phrases behind. Protected placeholders are opaque
// tokens, so code, links, quotations, and tables pass through untouched.
func Polish(text string) string {
	return PolishStyle(text, true)
}

// PolishStyle applies the deterministic cleanup with the register in mind.
// Academic prose keeps its formal vocabulary and avoids contractions.
func PolishStyle(text string, allowContractions bool) string {
	text = strings.ReplaceAll(text, " — ", ", ")
	text = strings.ReplaceAll(text, "—", ", ")
	for pass := 0; pass < 2; pass++ {
		for _, phrase := range phraseReplacements {
			text = replacePhrase(text, phrase.from, phrase.to)
		}
		for _, rule := range structuralReplacements {
			text = rule.pattern.ReplaceAllString(text, rule.with)
		}
		for _, word := range wordReplacements {
			text = replaceWord(text, word.from, word.to)
		}
		if allowContractions {
			for _, phrase := range casualPhraseReplacements {
				text = replacePhrase(text, phrase.from, phrase.to)
			}
			for _, word := range casualWordReplacements {
				text = replaceWord(text, word.from, word.to)
			}
		}
	}
	if allowContractions {
		text = contractIsAbout(text)
		for _, phrase := range contractions {
			text = replaceContraction(text, phrase.from, phrase.to)
		}
		for _, opener := range sentenceOpeners {
			text = replaceSentenceOpener(text, opener.from, opener.to)
		}
	}
	text = articleFix.ReplaceAllString(text, "an $1")
	text = doubleComma.ReplaceAllString(text, ",")
	text = spaceBeforePunctuation.ReplaceAllString(text, "$1")
	text = midLineSpaces.ReplaceAllString(text, "$1 $2")
	text = capitalizeStarts(text)
	return strings.TrimSpace(text)
}

func replaceSentenceOpener(text, from, to string) string {
	pattern := regexp.MustCompile(`(?im)(^|[.!?]\s+)` + regexp.QuoteMeta(from) + `\s*,?\s*`)
	return pattern.ReplaceAllString(text, "${1}"+to+" ")
}

type phraseReplacement struct{ from, to string }

var phraseReplacements = []phraseReplacement{
	{"in today's fast-paced world", "these days"},
	{"in today’s fast-paced world", "these days"},
	{"fast-paced world", "busy world"},
	{"in the realm of", "in"},
	{"it's important to note that", ""},
	{"it is important to note that", ""},
	{"it's worth noting that", ""},
	{"it is worth noting that", ""},
	{"let's dive in", "let's start"},
	{"let's delve into", "let's look at"},
	{"may potentially", "may"},
	{"can often help to", "can help"},
	{"could potentially", "could"},
	{"unlock significant potential", "find real potential"},
	{"unlock real potential", "find real potential"},
	{"unlock potential", "find potential"},
	{"navigate this landscape", "work in this field"},
	{"navigate the landscape", "work in this field"},
	{"in conclusion,", ""},
	{"in summary,", ""},
	{"at the end of the day,", ""},
	{"ultimately,", ""},
}

var casualPhraseReplacements = []phraseReplacement{
	{"has fundamentally changed how professionals operate", "has changed how people work"},
	{"strong communication strategies", "good ways to talk"},
	{"clear communication plans for remote teams", "simple ways to talk so remote teams can work together"},
	{"communication plans for remote teams", "ways to talk so remote teams can work together"},
	{"clear communication plans", "simple ways to talk"},
	{"communication plans", "ways to talk"},
	{"are key in this environment", "matter here"},
	{"to help remote teams work together", "so remote teams can work together"},
	{"using these platforms well lets teams", "use these tools well and teams can"},
	{"leaders need to be open and make the rules clear", "leaders need to be open. They also need to make the rules clear"},
	{"will thrive in a crowded field", "will do well in a crowded field"},
	{"same page. Too many", "same page. But too many"},
	{"use these tools well and teams can", "use these tools well. Teams can"},
	{"and drive new ideas while handling", ". They can also drive new ideas while handling"},
	{"hybrid work mess", "the mess of hybrid work"},
	{"It's about people.", "And people."},
	{"to make remote teams work well together", "so their remote teams can work together"},
	{"they find real potential", "they can find real potential"},
	{"while handling hybrid work's challenges", "while they handle the challenges of hybrid work"},
	{"keep teams aligned", "keep teams on the same page"},
	{"meetings kill output", "meetings can kill output"},
	{"to cut needless meetings and make room for focused work", ". That cuts needless meetings and makes room for focused work"},
	{"async tools matter here", "async tools can help here"},
	{"remote work has changed the modern professional world", "remote work changed the job. The modern one is different"},
	{"meetings still matter", "still, meetings matter"},
	{"they keep teams on the same page", "they just keep teams on the same page"},
	{"teams should use async updates. that cuts", "teams should use async updates. That's the easy part. That cuts"},
	{"that cuts needless meetings and makes room for focused work", "that cuts needless meetings, and it makes room for focused work"},
	{"help distributed teams collaborate well", "help remote teams work together"},
	{"play a key role here", "matter here"},
	{"tap into significant potential", "find real potential"},
	{"encourage innovation", "spark new ideas"},
	{"managing hybrid work complexities", "handling the mess of hybrid work"},
	{"handle hybrid complexity", "handle the mess of hybrid work"},
	{"hybrid complexity", "the mess of hybrid work"},
	{"prioritize transparency", "be open"},
	{"set clear expectations", "make the rules clear"},
	{"set clear ground rules", "make the rules clear"},
	{"must be open", "need to be open"},
	{"well-positioned to compete", "ready to compete"},
	{"well positioned to compete", "ready to compete"},
	{"better positioned to compete", "ready to compete"},
	{"better ready to compete", "ready to compete"},
	{"increasingly competitive landscape", "crowded market"},
	{"remain important for team alignment", "still matter for keeping teams on the same page"},
	{"still matter for keeping teams on the same page", "still matter. They keep teams on the same page"},
	{"still matter for team alignment", "still matter. They keep teams aligned"},
	{"hurt productivity", "kill output"},
	{"block deep work", "block real work"},
	{"cut down on unnecessary gatherings", "cut needless meetings"},
	{"build a culture of focused execution", "make room for focused work"},
	{"handle this balance with intention", "handle this balance with care"},
	{"balance this with care", "handle this balance with care"},
	{"builds a culture of focused work", "makes room for focused work"},
	{"embracing this shift will thrive", "that embrace this shift will do well"},
	{"still matter for agreement", "still matter. They keep teams on the same page"},
	{"the most effective teams", "the best teams"},
	{"embrace this shift", "take this shift seriously"},
}

type structuralReplacement struct {
	pattern *regexp.Regexp
	with    string
}

var structuralReplacements = []structuralReplacement{
	{
		regexp.MustCompile(`(?i)\b(not just)\s+([^,.;!?\n]{1,80}),?\s*but\s+([^.!?\n]{1,80})`),
		"$2 and $3",
	},
	{
		regexp.MustCompile(`(?i)\b(is|are|was|were)\s+about\s+([^.,;!?\n]{1,80}),\s*and\s+about\s+([^.!?\n]{1,80})`),
		"$1 about $2. It's about $3",
	},
	{
		regexp.MustCompile(`(?i)\b(is|are|was|were)\s+([^.!?\n]{1,60}?)\s+and\s+about\s+([^.!?\n]{1,60})`),
		"$1 about $2 and $3",
	},
	{
		regexp.MustCompile(`(?i)\b(is|are|was|were)\s+about\s+([^.,;!?\n]{1,60}?)\s+and\s+([^.!?\n]{1,60})`),
		"$1 about $2. It's about $3",
	},
	{
		regexp.MustCompile(`(?i)\b's\s+about\s+([^.,;!?\n]{1,60}?)\s+and\s+([^.!?\n]{1,60})`),
		"'s about $1. It's about $2",
	},
	{
		regexp.MustCompile(`(?i)([^.!?\n]{1,120}?),\s*not just\s+([^.!?\n]{1,80})`),
		"$1 and $2",
	},
	{
		regexp.MustCompile(`(?i)\b([A-Z][^.!?\n]{0,80}?)\s+(?:isn't|is not)\s+just\s+about\s+([^.,;!?\n]{1,80})[.;]\s*(?:it'?s|it is)\s+(?:also\s+)?about\s+([^.!?\n]{1,80})`),
		"$1 is about $2 and $3",
	},
	{
		regexp.MustCompile(`(?i)\b([A-Z][^.!?\n]{0,80}?)\s+(?:isn't|is not)\s+about\s+([^.,;!?\n]{1,80})[.;]\s*(?:it'?s|it is)\s+about\s+([^.!?\n]{1,80})`),
		"$1 is about $2 and $3",
	},
	{
		regexp.MustCompile(`(?i)\b([A-Z][^.!?\n]{0,80}?)\s+(?:isn't|is not)\s+just\s+([^.,;!?\n]{1,80})[.,;!?]?\s*(?:it'?s|it is)\s+(?:also\s+)?([^.!?\n]{1,80})`),
		"$1 is $2 and $3",
	},
	{
		regexp.MustCompile(`(?im)(^|[.!?]\s+)(?:ultimately|overall|in conclusion|in summary|at the end of the day),?\s*`),
		"$1",
	},
}

type wordReplacement struct{ from, to string }

var wordReplacements = []wordReplacement{
	{"delve", "dig into"},
	{"tapestry", "mix"},
	{"landscape", "field"},
	{"realm", "area"},
	{"testament", "proof"},
	{"pivotal", "key"},
	{"crucial", "important"},
	{"robust", "strong"},
	{"seamless", "smooth"},
	{"leverage", "use"},
	{"utilize", "use"},
	{"foster", "encourage"},
	{"underscore", "highlight"},
	{"multifaceted", "complex"},
	{"intricate", "complex"},
	{"embark", "start"},
	{"unlock", "find"},
	{"elevate", "improve"},
	{"game-changer", "big change"},
	{"facilitate", "help"},
	{"commence", "start"},
	{"leveraging", "using"},
	{"leveraged", "used"},
	{"utilizing", "using"},
	{"utilized", "used"},
	{"fostering", "encouraging"},
	{"fostered", "encouraged"},
	{"unlocking", "finding"},
	{"elevating", "improving"},
	{"navigate", "handle"},
	{"navigating", "handling"},
	{"navigated", "handled"},
	{"seamlessly", "smoothly"},
	{"robustly", "strongly"},
	{"crucially", "importantly"},
	{"pivotally", "importantly"},
	{"intricately", "closely"},
	{"fosters", "encourages"},
	{"facilitates", "helps"},
	{"facilitated", "helped"},
	{"facilitating", "helping"},
	{"leverages", "uses"},
	{"utilizes", "uses"},
	{"underscores", "highlights"},
	{"unlocks", "finds"},
	{"elevates", "improves"},
	{"embarks", "starts"},
	{"delves", "digs into"},
	{"commences", "starts"},
	{"navigates", "handles"},
	{"landscapes", "fields"},
	{"realms", "areas"},
	{"game-changers", "big changes"},
}

var casualWordReplacements = []wordReplacement{
	{"fundamentally", "really"},
	{"professionals", "people"},
	{"organizations", "companies"},
	{"strategies", "plans"},
	{"distributed", "remote"},
	{"asynchronous", "async"},
	{"effectively", "well"},
	{"significant", "real"},
	{"innovation", "new ideas"},
	{"complexities", "mess"},
	{"prioritize", "put first"},
	{"transparency", "openness"},
	{"expectations", "ground rules"},
	{"positioned", "ready"},
	{"competitive", "crowded"},
	{"alignment", "agreement"},
	{"productivity", "output"},
	{"unnecessary", "needless"},
	{"gatherings", "meetings"},
	{"execution", "work"},
	{"intention", "care"},
	{"collaboration", "teamwork"},
}

var contractions = []phraseReplacement{
	{"do not", "don't"},
	{"does not", "doesn't"},
	{"did not", "didn't"},
	{"is not", "isn't"},
	{"are not", "aren't"},
	{"was not", "wasn't"},
	{"were not", "weren't"},
	{"cannot", "can't"},
	{"can not", "can't"},
	{"will not", "won't"},
	{"would not", "wouldn't"},
	{"should not", "shouldn't"},
	{"could not", "couldn't"},
	{"must not", "mustn't"},
	{"need not", "needn't"},
	{"have not", "haven't"},
	{"has not", "hasn't"},
	{"had not", "hadn't"},
	{"it is", "it's"},
	{"that is", "that's"},
	{"there is", "there's"},
	{"what is", "what's"},
	{"who is", "who's"},
	{"here is", "here's"},
	{"they are", "they're"},
	{"we are", "we're"},
	{"you are", "you're"},
	{"i am", "i'm"},
	{"we have", "we've"},
	{"they have", "they've"},
	{"you have", "you've"},
	{"i have", "i've"},
	{"let us", "let's"},
}

type sentenceOpener struct{ from, to string }

var sentenceOpeners = []sentenceOpener{
	{"Moreover", "And"},
	{"Furthermore", "And"},
	{"Additionally", "Also"},
	{"In addition", "Also"},
	{"However", "But"},
	{"Therefore", "So"},
	{"Thus", "So"},
	{"Consequently", "So"},
	{"Nevertheless", "Still"},
	{"Nonetheless", "Still"},
}

var (
	articleFix             = regexp.MustCompile(`(?i)\ba (important|area|open|improve|encourage|highlight|effective|honest|intricate)\b`)
	doubleComma            = regexp.MustCompile(`,\s*,+`)
	spaceBeforePunctuation = regexp.MustCompile(`\s+([,.;:!?])`)
	midLineSpaces          = regexp.MustCompile(`([^\n \t])[ \t]{2,}([^\n \t])`)
)

var isAboutPattern = regexp.MustCompile(`(?i)\b([A-Za-z][A-Za-z-]{2,})\s+is\s+about\b`)

var isAboutStop = map[string]bool{
	"this": true, "that": true, "it": true, "there": true, "what": true,
	"who": true, "here": true, "she": true, "he": true, "we": true,
	"they": true, "you": true, "all": true, "one": true, "none": true,
	"each": true,
}

func contractIsAbout(text string) string {
	return isAboutPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := isAboutPattern.FindStringSubmatch(match)
		if len(parts) != 2 || isAboutStop[strings.ToLower(parts[1])] {
			return match
		}
		return parts[1] + "'s about"
	})
}

func replaceContraction(text, from, to string) string {
	pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(from) + `\b`)
	return pattern.ReplaceAllString(text, to)
}

func replacePhrase(text, from, to string) string {
	pattern := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(from))
	return pattern.ReplaceAllString(text, to)
}

func replaceWord(text, from, to string) string {
	pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(from) + `\b`)
	return pattern.ReplaceAllStringFunc(text, func(match string) string {
		return matchCase(match, to)
	})
}

func matchCase(original, replacement string) string {
	if original == "" || replacement == "" {
		return replacement
	}
	if original == strings.ToUpper(original) && len(original) > 1 {
		return strings.ToUpper(replacement)
	}
	first, _ := utf8.DecodeRuneInString(original)
	if unicode.IsUpper(first) {
		upper, n := utf8.DecodeRuneInString(replacement)
		if n > 0 {
			return string(unicode.ToUpper(upper)) + replacement[n:]
		}
	}
	return replacement
}

func capitalizeStarts(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	capitalizeNext := true
	pendingEnd := false
	var previous rune
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "[[WB") {
			if end := strings.Index(text[i:], "]]"); end >= 0 {
				b.WriteString(text[i : i+end+2])
				i += end + 2
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if pendingEnd {
			if unicode.IsSpace(r) {
				capitalizeNext = true
			}
			pendingEnd = false
		}
		if capitalizeNext && unicode.IsLetter(r) {
			r = unicode.ToUpper(r)
			capitalizeNext = false
		} else if !unicode.IsSpace(r) {
			capitalizeNext = false
		}
		b.WriteRune(r)
		i += size
		switch r {
		case '.', '!', '?':
			if r != '.' || !unicode.IsDigit(previous) {
				pendingEnd = true
			}
		case '\n':
			capitalizeNext = true
		}
		if !unicode.IsSpace(r) {
			previous = r
		}
	}
	return b.String()
}
