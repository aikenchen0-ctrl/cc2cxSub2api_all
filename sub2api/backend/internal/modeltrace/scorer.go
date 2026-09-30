// Package modeltrace embeds and evaluates the ModelTrace unified fingerprint bank.
// The source project is MIT licensed; see LICENSE.ModelTrace in this directory.
package modeltrace

import (
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"sync"
	"unicode"
)

const (
	ValueMin  = 1
	ValueMax  = 355
	Dimension = ValueMax - ValueMin + 1
	alpha     = 0.5
)

//go:embed unified_bank.json
var bankJSON []byte

type Bank struct {
	BuiltAt     string                 `json:"built_at"`
	Models      []BankModel            `json:"models"`
	Robust      RobustArtifacts        `json:"robust"`
	Calibration map[string]Calibration `json:"calibration"`
}

type BankModel struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Family      string    `json:"family"`
	FamilyName  string    `json:"family_name"`
	Counts      []float64 `json:"counts"`
}

type RobustArtifacts struct {
	Hellinger    FeatureArtifact `json:"hellinger"`
	OrderedBlock OrderedArtifact `json:"ordered_blocks"`
}

type FeatureArtifact struct {
	FeatureMean   []float64   `json:"feature_mean"`
	FeatureScale  []float64   `json:"feature_scale"`
	NuisanceBasis [][]float64 `json:"nuisance_basis"`
	Centroids     [][]float64 `json:"centroids"`
}

type OrderedArtifact struct {
	FeatureArtifact
	EnvironmentCentroids [][][]float64 `json:"environment_centroids"`
	Weight               float64       `json:"weight"`
}

func (a *OrderedArtifact) UnmarshalJSON(data []byte) error {
	var value struct {
		FeatureMean          []float64     `json:"feature_mean"`
		FeatureScale         []float64     `json:"feature_scale"`
		NuisanceBasis        [][]float64   `json:"nuisance_basis"`
		Centroids            [][]float64   `json:"centroids"`
		EnvironmentCentroids [][][]float64 `json:"environment_centroids"`
		Weight               float64       `json:"weight"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	a.FeatureArtifact = FeatureArtifact{FeatureMean: value.FeatureMean, FeatureScale: value.FeatureScale, NuisanceBasis: value.NuisanceBasis, Centroids: value.Centroids}
	a.EnvironmentCentroids = value.EnvironmentCentroids
	a.Weight = value.Weight
	return nil
}

type Calibration struct {
	Beta       float64 `json:"beta"`
	CVAccuracy float64 `json:"cv_accuracy"`
}

type Output struct {
	Text          string `json:"text"`
	ExpectedCount int    `json:"expected_count"`
}

type Diagnostic struct {
	Index          int  `json:"index"`
	ParsedNumbers  int  `json:"parsed_numbers"`
	MinimumNumbers int  `json:"minimum_numbers"`
	Accepted       bool `json:"accepted"`
}

type Result struct {
	Model                  string  `json:"model"`
	DisplayName            string  `json:"display_name"`
	Probability            float64 `json:"probability"`
	ConditionalProbability float64 `json:"conditional_probability"`
	ProfileSimilarity      float64 `json:"profile_similarity"`
	Score                  float64 `json:"score"`
	Family                 string  `json:"family"`
	FamilyName             string  `json:"family_name"`
}

type FamilyResult struct {
	Family      string  `json:"family"`
	DisplayName string  `json:"display_name"`
	Probability float64 `json:"probability"`
}

type Analysis struct {
	Prediction           string         `json:"prediction"`
	PredictionName       string         `json:"prediction_name"`
	Probability          float64        `json:"probability"`
	UsedOutputs          int            `json:"used_outputs"`
	Results              []Result       `json:"results"`
	Diagnostics          []Diagnostic   `json:"diagnostics"`
	FamilyPrediction     string         `json:"family_prediction"`
	FamilyPredictionName string         `json:"family_prediction_name"`
	FamilyProbability    float64        `json:"family_probability"`
	FamilyProbabilities  []FamilyResult `json:"family_probabilities"`
	Method               string         `json:"method"`
}

type Challenge struct {
	ID            string `json:"id"`
	ExpectedCount int    `json:"expected_count"`
	Prompt        string `json:"prompt"`
}

var (
	bankOnce sync.Once
	bankData *Bank
	bankErr  error
	digitsRE = regexp.MustCompile(`\d+`)
)

func LoadBank() (*Bank, error) {
	bankOnce.Do(func() {
		var bank Bank
		if err := json.Unmarshal(bankJSON, &bank); err != nil {
			bankErr = err
			return
		}
		if len(bank.Models) == 0 || len(bank.Robust.Hellinger.Centroids) != len(bank.Models) {
			bankErr = errors.New("invalid ModelTrace bank")
			return
		}
		bankData = &bank
	})
	return bankData, bankErr
}

func SupportedModels() []string {
	bank, err := LoadBank()
	if err != nil {
		return nil
	}
	models := make([]string, 0, len(bank.Models))
	for _, model := range bank.Models {
		models = append(models, model.ID)
	}
	return models
}

func GenerateChallenges(count int) []Challenge {
	if count <= 0 {
		count = 3
	}
	lengths := randomSample(292, 332, count)
	challenges := make([]Challenge, 0, count)
	for i, length := range lengths {
		challenges = append(challenges, Challenge{
			ID:            "probe-" + itoa(i+1) + "-" + randomHex(7),
			ExpectedCount: length,
			Prompt:        "这是一次独立的数值选择记录。请逐项凭第一反应选择 " + itoa(length) + " 个 1 到 355（含端点）的整数。每个位置都要单独选择；不要从 1 开始计数，不要连续递增或递减，也不要采用等差、循环、重复区块或其他规则化模式。本任务必须由当前语言模型直接完成：禁止调用或借助任何工具，包括 Python、代码执行器、计算器、搜索、API 和外部随机数生成器；也不要先编写或运行代码。允许数字重复；不要排序、去重或修改已经写出的项目。数字之间用逗号或空格分隔。直接从第一个取值开始输出，不要解释。",
		})
	}
	return challenges
}

func Analyze(outputs []Output) (*Analysis, error) {
	bank, err := LoadBank()
	if err != nil {
		return nil, err
	}
	type validOutput struct {
		numbers []int
		counts  []float64
		scores  []float64
	}
	valid := make([]validOutput, 0, len(outputs))
	diagnostics := make([]Diagnostic, 0, len(outputs))
	for index, output := range outputs {
		numbers := ParseNumbers(output.Text)
		minimum := 80
		if output.ExpectedCount > 0 {
			minimum = maxInt(80, int(math.Ceil(float64(output.ExpectedCount)*0.55)))
		}
		accepted := len(numbers) >= minimum
		diagnostics = append(diagnostics, Diagnostic{Index: index, ParsedNumbers: len(numbers), MinimumNumbers: minimum, Accepted: accepted})
		if accepted {
			counts := countNumbers(numbers)
			valid = append(valid, validOutput{numbers: numbers, counts: counts, scores: robustScoreNumbers(numbers, bank)})
		}
	}
	if len(valid) == 0 {
		return nil, errors.New("没有可用回答：模型拒答或数字序列严重截断")
	}
	combined := make([]float64, len(bank.Models))
	for modelIndex := range combined {
		values := make([]float64, len(valid))
		for i := range valid {
			values[i] = valid[i].scores[modelIndex]
		}
		combined[modelIndex] = mean(values)
	}
	calibrationKey := itoa(minInt(len(valid), 3))
	calibration, ok := bank.Calibration[calibrationKey]
	if !ok {
		return nil, errors.New("missing ModelTrace calibration")
	}
	scaled := make([]float64, len(combined))
	for i := range combined {
		scaled[i] = calibration.Beta * combined[i]
	}
	probabilities := softmax(scaled)
	pooled := make([]float64, Dimension)
	for _, item := range valid {
		for i, value := range item.counts {
			pooled[i] += value
		}
	}
	results := make([]Result, 0, len(bank.Models))
	familyOrder := make([]string, 0, 2)
	familyNames := map[string]string{}
	for i, model := range bank.Models {
		family := model.Family
		if family == "" {
			family = "models"
		}
		if _, exists := familyNames[family]; !exists {
			familyOrder = append(familyOrder, family)
			familyNames[family] = model.FamilyName
		}
		results = append(results, Result{Model: model.ID, DisplayName: model.DisplayName, Probability: probabilities[i], ProfileSimilarity: jsSimilarity(pooled, model.Counts), Score: combined[i], Family: family, FamilyName: familyNames[family]})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Probability > results[j].Probability })
	familyProbabilities := map[string]float64{}
	for _, result := range results {
		familyProbabilities[result.Family] += result.Probability
	}
	for i := range results {
		results[i].ConditionalProbability = results[i].Probability / familyProbabilities[results[i].Family]
	}
	winningFamily := familyOrder[0]
	familyResults := make([]FamilyResult, 0, len(familyOrder))
	for _, family := range familyOrder {
		if familyProbabilities[family] > familyProbabilities[winningFamily] {
			winningFamily = family
		}
		familyResults = append(familyResults, FamilyResult{Family: family, DisplayName: familyNames[family], Probability: familyProbabilities[family]})
	}
	return &Analysis{Prediction: results[0].Model, PredictionName: results[0].DisplayName, Probability: results[0].Probability, UsedOutputs: len(valid), Results: results, Diagnostics: diagnostics, FamilyPrediction: winningFamily, FamilyPredictionName: familyNames[winningFamily], FamilyProbability: familyProbabilities[winningFamily], FamilyProbabilities: familyResults, Method: "统一全局稳健数字指纹"}, nil
}

func ParseNumbers(text string) []int {
	matches := digitsRE.FindAllStringIndex(text, -1)
	runs := make([][]int, 0)
	current := make([]int, 0)
	previousEnd := 0
	for _, match := range matches {
		separator := text[previousEnd:match[0]]
		if len(current) > 0 && containsLetter(separator) {
			runs = append(runs, current)
			current = make([]int, 0)
		}
		value := atoi(text[match[0]:match[1]])
		if value >= ValueMin && value <= ValueMax {
			current = append(current, value)
		}
		previousEnd = match[1]
	}
	if len(current) > 0 {
		runs = append(runs, current)
	}
	best := []int{}
	for _, run := range runs {
		if len(run) > len(best) {
			best = run
		}
	}
	return best
}

func robustScoreNumbers(numbers []int, bank *Bank) []float64 {
	marginal := robustScoreCounts(countNumbers(numbers), bank)
	if bank.Robust.OrderedBlock.Weight == 0 || len(bank.Robust.OrderedBlock.Centroids) == 0 {
		return marginal
	}
	ordered := orderedBlockScores(numbers, bank)
	for i := range marginal {
		marginal[i] = (1-bank.Robust.OrderedBlock.Weight)*marginal[i] + bank.Robust.OrderedBlock.Weight*ordered[i]
	}
	return marginal
}

func robustScoreCounts(counts []float64, bank *Bank) []float64 {
	a := bank.Robust.Hellinger
	feature := hellingerFeature(counts)
	projected := make([]float64, len(feature))
	for i := range feature {
		projected[i] = (feature[i] - a.FeatureMean[i]) / a.FeatureScale[i]
	}
	projected = normalized(subtractBasis(projected, a.NuisanceBasis))
	scores := make([]float64, len(a.Centroids))
	for i, centroid := range a.Centroids {
		scores[i] = dot(projected, centroid)
	}
	return standardize(scores)
}

func orderedBlockScores(numbers []int, bank *Bank) []float64 {
	a := bank.Robust.OrderedBlock
	feature := orderedBlockFeature(numbers)
	standardized := make([]float64, len(feature))
	for i := range feature {
		standardized[i] = (feature[i] - a.FeatureMean[i]) / a.FeatureScale[i]
	}
	unit := normalized(standardized)
	templateRaw := make([]float64, len(a.Centroids))
	for modelIndex := range templateRaw {
		templateRaw[modelIndex] = math.Inf(-1)
		for _, environment := range a.EnvironmentCentroids {
			score := dot(unit, environment[modelIndex])
			if score > templateRaw[modelIndex] {
				templateRaw[modelIndex] = score
			}
		}
	}
	template := standardize(templateRaw)
	projected := normalized(subtractBasis(standardized, a.NuisanceBasis))
	nuisanceRaw := make([]float64, len(a.Centroids))
	for i, centroid := range a.Centroids {
		nuisanceRaw[i] = dot(projected, centroid)
	}
	nuisance := standardize(nuisanceRaw)
	combined := make([]float64, len(template))
	for i := range combined {
		combined[i] = 0.5*template[i] + 0.5*nuisance[i]
	}
	return standardize(combined)
}

func orderedBlockFeature(numbers []int) []float64 {
	pieces := make([]float64, 0, 74)
	base, remainder, start := len(numbers)/4, len(numbers)%4, 0
	for chunkIndex := 0; chunkIndex < 4; chunkIndex++ {
		size := base
		if chunkIndex < remainder {
			size++
		}
		bins := make([]float64, 16)
		for i := range bins {
			bins[i] = 0.5
		}
		for _, value := range numbers[start : start+size] {
			index := minInt(15, int(math.Floor((float64(value-1)/355)*16)))
			bins[index]++
		}
		start += size
		total := sum(bins)
		for _, value := range bins {
			pieces = append(pieces, math.Sqrt(value/total))
		}
	}
	lastDigits := make([]float64, 10)
	for i := range lastDigits {
		lastDigits[i] = 0.5
	}
	for _, value := range numbers {
		lastDigits[value%10]++
	}
	total := sum(lastDigits)
	for _, value := range lastDigits {
		pieces = append(pieces, math.Sqrt(value/total))
	}
	return pieces
}

func countNumbers(numbers []int) []float64 {
	counts := make([]float64, Dimension)
	for _, number := range numbers {
		counts[number-ValueMin]++
	}
	return counts
}
func hellingerFeature(counts []float64) []float64 {
	total := sum(counts) + alpha*Dimension
	out := make([]float64, len(counts))
	for i, v := range counts {
		out[i] = math.Sqrt((v + alpha) / total)
	}
	return out
}
func subtractBasis(values []float64, basis [][]float64) []float64 {
	out := append([]float64(nil), values...)
	for _, vector := range basis {
		projection := dot(out, vector)
		for i := range out {
			out[i] -= projection * vector[i]
		}
	}
	return out
}
func normalized(values []float64) []float64 {
	scale := math.Max(math.Sqrt(dot(values, values)), 1e-12)
	out := make([]float64, len(values))
	for i, v := range values {
		out[i] = v / scale
	}
	return out
}
func standardize(values []float64) []float64 {
	center := mean(values)
	variance := 0.0
	for _, v := range values {
		variance += (v - center) * (v - center)
	}
	variance /= float64(len(values))
	scale := math.Max(math.Sqrt(variance), 1e-12)
	out := make([]float64, len(values))
	for i, v := range values {
		out[i] = (v - center) / scale
	}
	return out
}
func softmax(values []float64) []float64 {
	maximum := values[0]
	for _, v := range values[1:] {
		if v > maximum {
			maximum = v
		}
	}
	out := make([]float64, len(values))
	total := 0.0
	for i, v := range values {
		out[i] = math.Exp(v - maximum)
		total += out[i]
	}
	for i := range out {
		out[i] /= total
	}
	return out
}
func jsSimilarity(left, right []float64) float64 {
	leftTotal := sum(left)
	rightTotal := sum(right) + alpha*Dimension
	js := 0.0
	for i := range left {
		p := left[i] / leftTotal
		q := (right[i] + alpha) / rightTotal
		midpoint := (p + q) / 2
		if p > 0 {
			js += 0.5 * p * math.Log(p/midpoint)
		}
		if q > 0 {
			js += 0.5 * q * math.Log(q/midpoint)
		}
	}
	return 1 - math.Sqrt(js/math.Log(2))
}
func dot(left, right []float64) float64 {
	value := 0.0
	for i := range left {
		value += left[i] * right[i]
	}
	return value
}
func mean(values []float64) float64 { return sum(values) / float64(len(values)) }
func sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}
func containsLetter(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}
func randomSample(min, max, count int) []int {
	values := make([]int, max-min+1)
	for i := range values {
		values[i] = min + i
	}
	for i := 0; i < len(values)-1; i++ {
		j := i + secureInt(len(values)-i)
		values[i], values[j] = values[j], values[i]
	}
	if count > len(values) {
		count = len(values)
	}
	return values[:count]
}
func secureInt(max int) int {
	if max <= 1 {
		return 0
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	return int(binary.LittleEndian.Uint64(b[:]) % uint64(max))
}
func randomHex(bytesCount int) string {
	const alphabet = "0123456789abcdef"
	out := make([]byte, bytesCount*2)
	for i := range out {
		out[i] = alphabet[secureInt(len(alphabet))]
	}
	return string(out)
}
func atoi(value string) int {
	n := 0
	for _, r := range value {
		n = n*10 + int(r-'0')
	}
	return n
}
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [24]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
