package modeltrace

import (
	"math"
	"strings"
	"testing"
)

func TestEmbeddedBankAndSupportedModels(t *testing.T) {
	bank, err := LoadBank()
	if err != nil {
		t.Fatal(err)
	}
	if len(bank.Models) != 16 {
		t.Fatalf("model count = %d, want 16", len(bank.Models))
	}
	models := SupportedModels()
	if models[0] != "gpt-5.4" || models[len(models)-1] != "claude-opus-5-5" {
		t.Fatalf("unexpected supported models: %v", models)
	}
}

func TestAnalyzeMatchesReferenceScorer(t *testing.T) {
	values := make([]string, 310)
	for i := range values {
		values[i] = itoa((i*73+41)%355 + 1)
	}
	text := strings.Join(values, ",")
	analysis, err := Analyze([]Output{
		{Text: text, ExpectedCount: 310},
		{Text: text, ExpectedCount: 310},
		{Text: text, ExpectedCount: 310},
	})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Prediction != "claude-opus-5-5" || analysis.FamilyPrediction != "claude" || analysis.UsedOutputs != 3 {
		t.Fatalf("unexpected analysis: %#v", analysis)
	}
	if math.Abs(analysis.Probability-0.719175183093423) > 1e-12 {
		t.Fatalf("probability = %.15f", analysis.Probability)
	}
	if math.Abs(analysis.Results[0].ProfileSimilarity-0.7354146660732197) > 1e-12 {
		t.Fatalf("similarity = %.15f", analysis.Results[0].ProfileSimilarity)
	}
}

func TestParseNumbersUsesLongestDigitRun(t *testing.T) {
	numbers := ParseNumbers("说明 12 13 14，然后 answer: 1, 2, 355, 9")
	if len(numbers) != 4 || numbers[2] != 355 {
		t.Fatalf("unexpected numbers: %v", numbers)
	}
}
