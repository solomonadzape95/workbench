package humanize

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/solomonadzape95/workbench/internal/config"
)

//go:embed rules.md
var embeddedRules string

// LoadRules returns the humanizer rules. A path set on the operation wins, then
// WORKBENCH_HUMANIZER_RULES, then the rules embedded at build time. Keeping the
// embedded copy means the binary works with no files beside it.
func LoadRules(path string) (string, error) {
	if strings.TrimSpace(path) != "" {
		data, err := os.ReadFile(config.ExpandHome(path))
		if err != nil {
			return "", fmt.Errorf("cannot read rules file: %w", err)
		}
		if strings.TrimSpace(string(data)) == "" {
			return "", fmt.Errorf("rules file %s is empty", path)
		}
		return string(data), nil
	}
	if env := strings.TrimSpace(os.Getenv("WORKBENCH_HUMANIZER_RULES")); env != "" {
		data, err := os.ReadFile(config.ExpandHome(env))
		if err != nil {
			return "", fmt.Errorf("cannot read WORKBENCH_HUMANIZER_RULES: %w", err)
		}
		if strings.TrimSpace(string(data)) == "" {
			return "", fmt.Errorf("WORKBENCH_HUMANIZER_RULES points at an empty file")
		}
		return string(data), nil
	}
	return embeddedRules, nil
}

// CoreRules returns the part of the rules document before the first style
// module. That section carries the rhythm, vocabulary, structure, punctuation,
// voice, imperfection, and safeguard rules.
func CoreRules(rules string) string {
	upper := strings.ToUpper(rules)
	idx := strings.Index(upper, "STYLE:")
	if idx < 0 {
		return strings.TrimSpace(rules)
	}
	return strings.TrimSpace(rules[:idx])
}

// StyleModule returns one STYLE block, without the following style. Style names
// are case-insensitive.
func StyleModule(rules, name string) string {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	upper := strings.ToUpper(rules)
	marker := "STYLE: " + name
	idx := strings.Index(upper, marker)
	if idx < 0 {
		return ""
	}
	start := idx + len(marker)
	rest := rules[start:]
	if next := strings.Index(strings.ToUpper(rest), "STYLE:"); next >= 0 {
		rest = rest[:next]
	}
	return strings.TrimSpace(rules[idx:start] + rest)
}

const casualRhythmContract = `RHYTHM CONTRACT
- Aim for an average sentence length of 12-18 words. Keep most sentences under 20.
- Include at least one sentence under 6 words per 150 words.
- Include at least one fragment per 200 words. A fragment like "Not always." or "Every time." is enough.
- Short sentences do not mean fewer points. Keep every idea, detail, example, and qualification from the input.
- Never write three sentences in a row with similar lengths. After a sentence over 25 words, write one under 10.
- Start some sentences with And, But, So, Then, or Still.
- Use contractions: don't, can't, it's, we're, that's, isn't.
- Use plain one- and two-syllable words where you can. Use pronouns (you, we, I, it) and small joining words.
- Do not open sentences with Moreover, Furthermore, Additionally, However, Therefore, Thus, Consequently, Ultimately, Overall, or In conclusion.
- Use at most one em dash in the whole chunk; prefer a comma or a period.
- If the text has three or more paragraphs, make one of them a single sentence.
- Allow one mild aside or opinion per 200 words if the style permits.
`

const formalRhythmContract = `RHYTHM CONTRACT (formal)
- Aim for an average sentence length of 15-20 words.
- Mix short declarative claims with longer qualified sentences. Never write three sentences in a row with similar lengths.
- After a sentence over 25 words, write one under 12.
- Keep the formal register: no contractions, no slang, no fragments, no rhetorical questions.
- Keep the input's vocabulary when it is already clear. Do not replace plain words with longer synonyms just to sound academic.
- Break noun-heavy sentences into shorter claims.
- Vary transitions. Do not open with Moreover, Furthermore, Additionally, In addition, However, Therefore, Thus, or Consequently.
- Avoid em dashes; use commas, periods, or parentheses.
- Use plain words and concrete verbs. Avoid inflated abstractions.
`

func rhythmContractFor(styleText string) string {
	if strings.Contains(strings.ToLower(styleText), "style: academic") {
		return formalRhythmContract
	}
	return casualRhythmContract
}

// BuildPrompt assembles the editor instructions, the chosen style, an optional
// voice sample, and the text to rewrite.
func BuildPrompt(core, style, voice, protectedNote, previous, text string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(core))
	if style = strings.TrimSpace(style); style != "" {
		b.WriteString("\n\n")
		b.WriteString(style)
	}
	if voice = strings.TrimSpace(voice); voice != "" {
		b.WriteString("\n\nVOICE SAMPLE\n")
		b.WriteString("Match the voice of this sample. Do not copy its content or add facts from it.\n")
		b.WriteString(truncateRunes(voice, 6000))
	}

	b.WriteString("\n\n")
	b.WriteString(rhythmContractFor(style))
	b.WriteString("\nOUTPUT CONTRACT\n")
	b.WriteString("- Return only the rewritten text. No title, preamble, notes, explanation, or code fences.\n")
	b.WriteString("- Keep the input's paragraph breaks and Markdown structure. Do not add new headings, bullet points, bold text, or links, and do not remove existing ones.\n")
	b.WriteString("- Preserve names, numbers, dates, citations, quotations, technical terms, and code exactly.\n")
	b.WriteString("- Every token like [[WB0]] is protected data. Keep every one exactly where it is. Never translate, remove, duplicate, reorder, or move one, even if it marks a heading, title, or code block.\n")
	b.WriteString("- Do not summarize, merge, or delete sentences. Keep every example and qualification. Rewrite each point in place and keep the length within 15% of the input.\n")
	b.WriteString("- Before returning, check your draft for the banned words and phrasing in the rules above and remove them.\n")
	if protectedNote = strings.TrimSpace(protectedNote); protectedNote != "" {
		b.WriteString("\n")
		b.WriteString(protectedNote)
		b.WriteString("\n")
	}

	if previous = strings.TrimSpace(previous); previous != "" {
		b.WriteString("\nPREVIOUS PARAGRAPH (context only; do not repeat or rewrite it):\n")
		b.WriteString(previous)
		b.WriteString("\n")
	}
	b.WriteString("\nTEXT TO REWRITE:\n")
	b.WriteString(text)
	return b.String()
}

// RevisionPrompt asks for a fresh rewrite of the original text with explicit
// notes about what went wrong in the first attempt.
func RevisionPrompt(core, style, voice, protectedNote, text string, issues []string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(core))
	if style = strings.TrimSpace(style); style != "" {
		b.WriteString("\n\n")
		b.WriteString(style)
	}
	if voice = strings.TrimSpace(voice); voice != "" {
		b.WriteString("\n\nVOICE SAMPLE\n")
		b.WriteString("Match the voice of this sample. Do not copy its content.\n")
		b.WriteString(truncateRunes(voice, 4000))
	}
	b.WriteString("\n\n")
	b.WriteString(rhythmContractFor(style))
	b.WriteString("\nREVISION TASK\n")
	b.WriteString("A previous attempt had these problems:\n")
	for _, issue := range issues {
		b.WriteString("- ")
		b.WriteString(issue)
		b.WriteString("\n")
	}
	b.WriteString("\nRewrite the ORIGINAL TEXT again. Fix every listed problem. ")
	b.WriteString("Keep the meaning, facts, names, numbers, quotations, technical terms, and placeholders exactly. ")
	b.WriteString("Do not summarize or merge sentences; rewrite every point in place. Keep the paragraph breaks and Markdown structure. Keep the length within 15% of the original. ")
	b.WriteString("Return only the rewritten text. No title, preamble, notes, or code fences.\n")
	if protectedNote = strings.TrimSpace(protectedNote); protectedNote != "" {
		b.WriteString("\n")
		b.WriteString(protectedNote)
		b.WriteString("\n")
	}
	b.WriteString("\nORIGINAL TEXT:\n")
	b.WriteString(text)
	return b.String()
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
