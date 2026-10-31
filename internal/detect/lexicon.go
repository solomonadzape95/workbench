package detect

import "strings"

func wordSet(text string) map[string]bool {
	out := map[string]bool{}
	for _, word := range strings.Fields(text) {
		out[strings.ToLower(word)] = true
	}
	return out
}

var functionWords = wordSet(`
a an the and or but nor so yet for as if then than that this these those
i me my mine you your yours he him his she her hers it its we us our ours
they them their theirs who whom whose which what where when why how
am is are was were be been being have has had having do does did doing
can could may might must shall should will would ought
at by from in into of off on onto out over per to up upon with within without
about above across after against along among around before behind below beneath
beside between beyond during except inside near outside since through throughout
toward towards under until via while because although though unless whereas
not no never very too also just only even still already almost enough
there here where wherever whenever however therefore thus hence moreover
furthermore additionally consequently meanwhile nevertheless nonetheless
`)

var pronouns = wordSet(`
i me my mine myself you your yours yourself he him his himself she her hers herself
it its itself we us our ours ourselves they them their theirs themselves
who whom whose which what
`)

var formalStarters = wordSet(`
however moreover furthermore additionally therefore thus consequently overall
ultimately nevertheless nonetheless in conclusion in summary
`)

var formalConnectors = wordSet(`
moreover furthermore additionally however therefore thus consequently
nevertheless nonetheless in addition in conclusion in summary overall
ultimately
`)

var researchRegister = wordSet(`
study studies research survey surveys interview interviews questionnaire questionnaires
participant participants respondent respondents sample samples data dataset datasets
analysis analyses finding findings method methods methodology empirical qualitative
quantitative hypothesis hypotheses experiment experiments observation observations
measurement measurements
`)

var hedges = wordSet(`
may might can could often typically generally usually likely unlikely
suggests suggest indicates indicate appears appear tends tend relatively
somewhat arguably potentially possibly perhaps approximately
`)

type weightedPhrase struct {
	phrase string
	weight float64
}

// aiPhrases are the vocabulary and template signals used by the main score.
// These are deliberately phrase-level. A single common word such as "important"
// is not evidence; "it is important to note" is.
var aiPhrases = []weightedPhrase{
	{"in today's fast-paced world", 4},
	{"in today’s fast-paced world", 4},
	{"fast-paced world", 3},
	{"it is important to note", 3},
	{"it's important to note", 3},
	{"it is worth noting", 2.5},
	{"it's worth noting", 2.5},
	{"in the realm of", 2.5},
	{"in the ever-evolving", 2.5},
	{"ever-evolving", 1.5},
	{"delve into", 2.5},
	{"delve", 2.5},
	{"delves", 2.5},
	{"delving", 2.5},
	{"tapestry", 3},
	{"testament to", 2.5},
	{"a testament to", 2.5},
	{"pivotal", 2},
	{"crucial", 1.5},
	{"robust", 1.5},
	{"seamless", 2},
	{"leverage", 1.5},
	{"utilize", 1.5},
	{"utilise", 1.5},
	{"foster", 1.5},
	{"underscore", 1.8},
	{"underscores", 1.8},
	{"multifaceted", 2.5},
	{"intricate", 1.8},
	{"embark", 1.8},
	{"unlock", 1.2},
	{"elevate", 1.2},
	{"game-changer", 2.5},
	{"navigate the landscape", 2.5},
	{"navigate", 0.8},
	{"landscape", 1},
	{"realm", 1.5},
	{"moreover", 1.2},
	{"furthermore", 1.2},
	{"additionally", 1},
	{"in conclusion", 2.5},
	{"in summary", 1.8},
	{"overall,", 1.5},
	{"ultimately,", 0.8},
	{"plays a crucial role", 2.5},
	{"has significant implications", 2},
	{"cutting-edge", 1.2},
	{"holistic", 1.2},
	{"synergy", 2},
	{"paradigm", 1.2},
	{"transformative", 1.2},
	{"revolutionize", 1.2},
	{"empower", 1},
	{"streamline", 1},
	{"best-in-class", 1.5},
	{"circle back", 1.5},
	{"touch base", 1.5},
	{"it is essential to", 1.5},
	{"it's essential to", 1.5},
	{"a wide range of", 0.8},
	{"a comprehensive approach", 1.5},
	{"comprehensive understanding", 1.5},
	{"navigate the complexities", 2},
	{"in the digital age", 2},
	{"the digital age", 1.5},
	{"the modern world", 1},
	{"the modern workplace", 1},
	{"meaningful outcomes", 1},
	{"sustainable success", 1.2},
	{"drive meaningful", 1.2},
	{"unlock significant potential", 2.5},
	{"significant potential", 1.2},
	{"at the end of the day", 2},
	{"not just", 0.5},
	{"not only", 0.4},
	{"a myriad of", 2},
	{"a plethora of", 2},
	{"plethora", 2},
	{"myriad", 1.5},
	{"in today's digital", 2.5},
	{"in today’s digital", 2.5},
	{"the key takeaway", 1.5},
	{"key takeaways", 1.5},
	{"let's dive", 2},
	{"dive into", 1},
	{"here's a breakdown", 2},
	{"i hope this helps", 2.5},
	{"i'd be happy to", 2.5},
	{"i’d be happy to", 2.5},
	{"let me know if", 1.8},
	{"great question", 2},
	{"as an ai", 3},
	{"as a large language model", 4},
	{"i cannot", 1.5},
	{"i can't", 1.5},
	{"i don't have personal", 3},
	{"i’m sorry", 1.2},
	{"i'm sorry", 1.2},
}

type familyFingerprint struct {
	Key     string
	Label   string
	Phrases []weightedPhrase
	Notes   string
}

var familyFingerprints = []familyFingerprint{
	{
		Key:   "chatgpt",
		Label: "ChatGPT / GPT family",
		Notes: "Classic assistant vocabulary, structured headings, and formal transitions.",
		Phrases: []weightedPhrase{
			{"delve", 3}, {"tapestry", 3}, {"testament", 2.5}, {"pivotal", 2.5},
			{"multifaceted", 3}, {"intricate", 2}, {"embark", 2}, {"unlock", 1.5},
			{"elevate", 1.5}, {"game-changer", 3}, {"it is important to note", 2.5},
			{"it's worth noting", 2.5}, {"in conclusion", 2.5}, {"overall", 1.5},
			{"moreover", 1.5}, {"furthermore", 1.5}, {"additionally", 1.2},
			{"here's", 1.5}, {"let's", 1.2}, {"i'd be happy to", 2},
			{"i hope this helps", 2}, {"certainly", 1.5}, {"great question", 1.5},
			{"as an ai language model", 4},
		},
	},
	{
		Key:   "claude",
		Label: "Claude / Anthropic family",
		Notes: "Nuanced caveats, collaborative phrasing, and the 'that said / at its core' pattern.",
		Phrases: []weightedPhrase{
			{"that said", 2.5}, {"at its core", 2.5}, {"the key insight", 2},
			{"in practice", 1.2}, {"more importantly", 1.5}, {"nuanced", 2.5},
			{"multifaceted", 1.8}, {"thoughtful", 1.5}, {"careful", 1},
			{"rather than", 1}, {"not just", 1}, {"it's not about", 2},
			{"the real question", 2}, {"what matters", 1.5}, {"i'd be happy to", 2},
			{"let me know", 1.5}, {"i'm happy to", 1.8}, {"i'm sorry", 1.5},
			{"i cannot", 1.5}, {"i don't have", 1.8}, {"i'm an ai", 2},
			{"it's worth noting", 1.5},
		},
	},
	{
		Key:   "gemini",
		Label: "Gemini / Google family",
		Notes: "Helpful-assistant openers, absolute agreement, and large-language-model disclaimers.",
		Phrases: []weightedPhrase{
			{"as a large language model", 4}, {"i'm unable", 2.5}, {"great question", 2.5},
			{"absolutely", 1.5}, {"i'm happy to help", 2.5}, {"i hope this helps", 1.5},
			{"here's", 1.2}, {"let's", 1}, {"certainly", 1.5}, {"in conclusion", 1.5},
			{"overall", 1.2}, {"moreover", 1}, {"additionally", 1}, {"furthermore", 1},
			{"i cannot", 1.2}, {"i can't", 1.2}, {"as an ai", 1.5},
		},
	},
	{
		Key:   "llama",
		Label: "Llama / Meta family",
		Notes: "Open-model assistant boilerplate and personal-knowledge disclaimers.",
		Phrases: []weightedPhrase{
			{"as an ai", 2.5}, {"i'm just an ai", 3}, {"i'm an ai", 2.5},
			{"i don't have personal", 3}, {"i cannot", 1.5}, {"i can't", 1.5},
			{"i'm sorry", 1.2}, {"i'd be happy to", 1.2}, {"here's", 1},
			{"let's", 1}, {"it is important to note", 1.2}, {"in conclusion", 1.2},
			{"overall", 1}, {"furthermore", 1},
		},
	},
	{
		Key:   "mistral",
		Label: "Mistral family",
		Notes: "Short assistant disclaimers and direct, neutral openers.",
		Phrases: []weightedPhrase{
			{"as an ai", 2.5}, {"i'm an ai", 2.5}, {"i don't have personal", 2.5},
			{"i cannot", 1.5}, {"i can't", 1.5}, {"i'm sorry", 1.2},
			{"here's", 1}, {"let's", 1}, {"in conclusion", 1}, {"overall", 1},
			{"however", 0.6}, {"certainly", 1},
		},
	},
	{
		Key:   "deepseek",
		Label: "DeepSeek family",
		Notes: "Assistant disclaimer phrases with structured, transition-heavy answers.",
		Phrases: []weightedPhrase{
			{"as an ai", 2.5}, {"i'm an ai", 2.5}, {"i don't have personal", 2.5},
			{"i cannot", 1.5}, {"i can't", 1.5}, {"here's", 1.2}, {"let's", 1.2},
			{"certainly", 1.2}, {"in conclusion", 1.2}, {"overall", 1},
			{"moreover", 1}, {"furthermore", 1}, {"additionally", 1},
		},
	},
	{
		Key:   "qwen",
		Label: "Qwen / Alibaba family",
		Notes: "Assistant boilerplate with formal, list-friendly structure.",
		Phrases: []weightedPhrase{
			{"as an ai", 2.5}, {"i'm an ai", 2.5}, {"i don't have personal", 2.5},
			{"i cannot", 1.5}, {"i can't", 1.5}, {"here's", 1.2}, {"let's", 1.2},
			{"in conclusion", 1.2}, {"overall", 1}, {"moreover", 1},
			{"furthermore", 1}, {"additionally", 1}, {"certainly", 1},
		},
	},
	{
		Key:   "grok",
		Label: "Grok / xAI family",
		Notes: "Conversational assistant openers and direct, informal transitions.",
		Phrases: []weightedPhrase{
			{"as an ai", 2}, {"i'm", 0.6}, {"here's", 1.2}, {"let's", 1.2},
			{"certainly", 1.2}, {"i don't have", 1.5}, {"i can't", 1.2},
			{"in conclusion", 1}, {"overall", 1},
		},
	},
}

var detectorReferences = []Reference{
	{
		Title:   "DetectGPT: Zero-Shot Machine-Generated Text Detection using Probability Curvature",
		Authors: "Mitchell, Lee, Khazatsky, Manning, Finn",
		Year:    "2023",
		URL:     "https://arxiv.org/abs/2301.11305",
	},
	{
		Title:   "GLTR: Statistical Detection and Visualization of Generated Text",
		Authors: "Gehrmann, Strobelt, Rush",
		Year:    "2019",
		URL:     "https://arxiv.org/abs/1906.04043",
	},
	{
		Title:   "Spotting LLMs With Binoculars: Zero-Shot Detection of Machine-Generated Text",
		Authors: "Hans, Schwarzschild, Cherepanova, Kazemi, Saha, Goldblum, Geiping, Goldstein",
		Year:    "2024",
		URL:     "https://arxiv.org/abs/2401.12070",
	},
	{
		Title:   "Fast-DetectGPT: Efficient Zero-Shot Detection of Machine-Generated Text via Conditional Probability Curvature",
		Authors: "Bao, Zhao, Teng, Yang, Zhang",
		Year:    "2023",
		URL:     "https://arxiv.org/abs/2310.05130",
	},
	{
		Title:   "RAID: A Shared Benchmark for Robust Evaluation of Machine-Generated Text Detectors",
		Authors: "Dugan, Hwang, Trhlik, Ludan, Zhu, Xu, Ippolito, Callison-Burch",
		Year:    "2024",
		URL:     "https://arxiv.org/abs/2405.07940",
	},
	{
		Title:   "Perplexity, burstiness, and statistical AI detection",
		Authors: "Tian (GPTZero)",
		Year:    "2023",
		URL:     "https://gptzero.me/news/perplexity-and-burstiness-what-is-it/",
	},
	{
		Title:   "New AI classifier for indicating AI-written text",
		Authors: "OpenAI",
		Year:    "2023",
		URL:     "https://openai.com/index/new-ai-classifier-for-indicating-ai-written-text/",
	},
	{
		Title:   "Turnitin's AI writing detection capabilities FAQs",
		Authors: "Turnitin",
		Year:    "2026",
		URL:     "https://guides.turnitin.com/hc/en-us/articles/28477544839821-Turnitin-s-AI-writing-detection-capabilities-FAQs",
	},
}

var detectorMethodology = []string{
	"Extracts prose only: headings, tables, figures, reference lists, code, blockquotes, and short labels are skipped.",
	"Scores sentence-level rhythm: length distribution, short-sentence share, uniform runs, paragraph variance, and neighbour similarity.",
	"Measures lexical and syntactic signals: function-word rate, pronouns, contractions, word length, starter diversity, formal openers, hedging, repetition, punctuation, and specificity.",
	"Adds an AI phrase/fingerprint layer for assistant boilerplate, template transitions, and known model-family vocabulary.",
	"Blends the document-level stylometric probability with the share of prose sentences above the sentence threshold.",
	"Reports every contributing signal with its raw value. The score is diagnostic, not proof of authorship.",
	"This is an explainable stylometric ensemble, not a trained neural classifier. It does not use your text for training.",
	"False positives are more likely on formulaic human writing, non-native English, legal/academic boilerplate, and heavily edited text. Paraphrasing and humanizing erase fingerprints.",
	"Model attribution is a best-effort phrase/style fingerprint. Models imitate each other, so an exact model cannot be proven from text alone.",
}
