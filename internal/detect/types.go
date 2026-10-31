// Package detect implements Workbench's explainable AI-writing detector.
//
// It is deliberately not a black-box classifier. The score is assembled from
// sentence-level stylometry, document rhythm, vocabulary, punctuation, repeated
// phrasing, and a fingerprint layer for known assistant artifacts. Every signal
// is reported with its raw value and contribution so the reader can see why the
// score came out the way it did.
package detect

import "time"

const Version = "1"

// Source is the file the detector read.
type Source struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Format string `json:"format"`
	Bytes  int64  `json:"bytes"`
}

// Summary is the short version the UI and job metadata use.
type Summary struct {
	AI              float64 `json:"ai_percent"`
	Human           float64 `json:"human_percent"`
	Verdict         string  `json:"verdict"`
	Confidence      string  `json:"confidence"`  // low, medium, high
	Reliability     float64 `json:"reliability"` // 0-100
	SentenceAI      float64 `json:"ai_sentence_percent"`
	Flagged         int     `json:"flagged_sentences"`
	Words           int     `json:"words"`
	Sentences       int     `json:"sentences"`
	Paragraphs      int     `json:"paragraphs"`
	AvgSentence     float64 `json:"avg_sentence_words"`
	SentenceCV      float64 `json:"sentence_cv"`
	Profile         string  `json:"profile"`
	AttributionTop  string  `json:"top_model,omitempty"`
	AttributionNote string  `json:"attribution_note,omitempty"`
}

// Signal is one diagnostic feature. AIScore is the direction-normalised
// AI-likeness (0-100), not the raw value.
type Signal struct {
	Key          string  `json:"key"`
	Label        string  `json:"label"`
	Group        string  `json:"group"`
	Raw          float64 `json:"raw"`
	Display      string  `json:"display"`
	AIScore      float64 `json:"ai_score"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
	Direction    string  `json:"direction"` // "AI-like" or "human-like"
	Detail       string  `json:"detail"`
}

// Paragraph is a paragraph-level rollup. Index is the order in the extracted
// prose, not the original document's XML index.
type Paragraph struct {
	Index     int     `json:"index"`
	Words     int     `json:"words"`
	Sentences int     `json:"sentences"`
	AI        float64 `json:"ai_percent"`
	Flagged   int     `json:"flagged"`
	Preview   string  `json:"preview"`
}

// Sentence is a flagged sentence with the reasons it crossed the threshold.
type Sentence struct {
	Index     int      `json:"index"`
	Paragraph int      `json:"paragraph"`
	Text      string   `json:"text"`
	Words     int      `json:"words"`
	AI        float64  `json:"ai_percent"`
	Reasons   []string `json:"reasons"`
}

// Attribution is a best-effort model-family fingerprint, not proof.
type Attribution struct {
	Family     string   `json:"family"`
	Label      string   `json:"label"`
	Score      float64  `json:"score"`      // 0-100 fingerprint strength
	Confidence string   `json:"confidence"` // low, medium, high
	Evidence   []string `json:"evidence,omitempty"`
	Notes      string   `json:"notes,omitempty"`
}

// Reference records the public methods and caveats the detector follows.
type Reference struct {
	Title   string `json:"title"`
	Authors string `json:"authors"`
	Year    string `json:"year"`
	URL     string `json:"url"`
}

// Stats records the raw document measurements for the report and JSON users.
type Stats struct {
	Profile           string  `json:"profile"`
	Words             int     `json:"words"`
	ContentWords      int     `json:"content_words"`
	Sentences         int     `json:"sentences"`
	Paragraphs        int     `json:"paragraphs"`
	AvgSentenceWords  float64 `json:"avg_sentence_words"`
	SentenceCV        float64 `json:"sentence_cv"`
	ShortSentenceRate float64 `json:"short_sentence_rate"`
	LongSentenceRate  float64 `json:"long_sentence_rate"`
	UniformRunRate    float64 `json:"uniform_run_rate"`
	ParagraphCV       float64 `json:"paragraph_cv"`
	FunctionWordRate  float64 `json:"function_word_rate"`
	PronounRate       float64 `json:"pronoun_rate"`
	ContractionRate   float64 `json:"contraction_rate"`
	AvgWordLength     float64 `json:"avg_word_length"`
	LongWordRate      float64 `json:"long_word_rate"`
	StarterDiversity  float64 `json:"starter_diversity"`
	FormalStarterRate float64 `json:"formal_starter_rate"`
	AIPhraseRate      float64 `json:"ai_phrase_rate"`
	AIPhraseWeight    float64 `json:"ai_phrase_weight"`
	AIPhraseHits      int     `json:"ai_phrase_hits"`
	FormalConnector   float64 `json:"formal_connector_rate"`
	RepeatedTrigrams  float64 `json:"repeated_trigram_rate"`
	EmDashRate        float64 `json:"em_dash_rate"`
	SemicolonRate     float64 `json:"semicolon_rate"`
	HedgeRate         float64 `json:"hedge_rate"`
	SpecificityRate   float64 `json:"specificity_rate"`
	ResearchRegister  float64 `json:"research_register_rate"`
	SentenceBuckets   [10]int `json:"sentence_buckets"`
}

// Report is the complete detector result. The Markdown report is generated
// from this structure; the JSON sidecar is this structure marshalled directly.
type Report struct {
	Version     string        `json:"version"`
	GeneratedAt time.Time     `json:"generated_at"`
	Source      Source        `json:"source"`
	Summary     Summary       `json:"summary"`
	Signals     []Signal      `json:"signals"`
	Paragraphs  []Paragraph   `json:"paragraphs"`
	Flagged     []Sentence    `json:"flagged_sentences"`
	Attribution []Attribution `json:"attribution"`
	Warnings    []string      `json:"warnings,omitempty"`
	Methodology []string      `json:"methodology"`
	References  []Reference   `json:"references"`
	Stats       Stats         `json:"stats"`
}
