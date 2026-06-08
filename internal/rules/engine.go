package rules

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"github.com/argus-platform/argus/internal/types"
)

type RuleSet struct {
	Rules []Rule `yaml:"rules"`
}

type Rule struct {
	Name string  `yaml:"name"`
	When WhenClause `yaml:"when"`
	Then ThenClause `yaml:"then"`
}

type WhenClause struct {
	MatchesRegex string   `yaml:"matches_regex,omitempty"`
	SourceIs     []string `yaml:"source_is,omitempty"`
	HasTag       []string `yaml:"has_tag,omitempty"`
	Confidence   string   `yaml:"confidence,omitempty"` // e.g. ">= 80", "60-79", "> 80"
}

type ThenClause struct {
	AddTag          []string `yaml:"add_tag,omitempty"`
	BoostConfidence int      `yaml:"boost_confidence,omitempty"`
	PublishTo       string   `yaml:"publish_to,omitempty"`
	TriggerAlertIf  string   `yaml:"trigger_alert_if,omitempty"`
}

type Engine struct {
	rules    []Rule
	regexMap map[string]*regexp.Regexp
}

func NewEngine(rulesPath string) (*Engine, error) {
	data, err := os.ReadFile(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("read rules: %w", err)
	}

	var rs RuleSet
	if err := yaml.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}

	regexMap := make(map[string]*regexp.Regexp)
	for i, r := range rs.Rules {
		if r.When.MatchesRegex != "" {
			re, err := regexp.Compile(r.When.MatchesRegex)
			if err != nil {
				return nil, fmt.Errorf("compile regex %q: %w", r.Name, err)
			}
			regexMap[rs.Rules[i].Name] = re
		}
	}

	return &Engine{rules: rs.Rules, regexMap: regexMap}, nil
}

func (e *Engine) Evaluate(data *types.EnrichedData) []types.RuleResult {
	var results []types.RuleResult

	for _, rule := range e.rules {
		result := types.RuleResult{
			Matched:  true,
			RuleName: rule.Name,
		}

		// Check content regex
		if re, ok := e.regexMap[rule.Name]; ok {
			if !re.MatchString(data.Content) && !re.MatchString(data.Title) {
				result.Matched = false
			}
		}

		// Check source filter
		if len(rule.When.SourceIs) > 0 {
			found := false
			for _, src := range rule.When.SourceIs {
				if strings.EqualFold(data.Source, src) {
					found = true
					break
				}
			}
			if !found {
				result.Matched = false
			}
		}

		// Check confidence condition
		if rule.When.Confidence != "" {
			if !matchConfidence(rule.When.Confidence, data.Confidence) {
				result.Matched = false
			}
		}

		if !result.Matched {
			continue
		}

		// Apply Then actions
		result.ConfidenceBoost = rule.Then.BoostConfidence
		if rule.Then.PublishTo != "" {
			result.TargetTopics = append(result.TargetTopics, rule.Then.PublishTo)
		}
		if rule.Then.TriggerAlertIf != "" {
			threshold := 80 // default
			if strings.HasPrefix(rule.Then.TriggerAlertIf, "> ") {
				if n, err := strconv.Atoi(strings.TrimPrefix(rule.Then.TriggerAlertIf, "> ")); err == nil {
					threshold = n
				}
			}
			if data.Confidence+rule.Then.BoostConfidence > threshold {
				result.TriggerAlert = true
			}
		}
		result.NeedsReview = data.Confidence+rule.Then.BoostConfidence >= 60 &&
			data.Confidence+rule.Then.BoostConfidence <= 79

		results = append(results, result)
	}

	return results
}

// matchConfidence checks if a confidence value satisfies a condition string.
// Supported formats: ">= 80", "> 80", "60-79", "< 50", "<= 50", "== 80"
func matchConfidence(cond string, val int) bool {
	cond = strings.TrimSpace(cond)

	// Range format: "60-79"
	if strings.Contains(cond, "-") && !strings.HasPrefix(cond, ">") && !strings.HasPrefix(cond, "<") {
		parts := strings.SplitN(cond, "-", 2)
		lo, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		hi, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 == nil && err2 == nil {
			return val >= lo && val <= hi
		}
	}

	// Comparison formats: >= 80, > 80, <= 50, < 50, == 80
	if strings.HasPrefix(cond, ">=") {
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(cond, ">=")))
		if err == nil { return val >= n }
	}
	if strings.HasPrefix(cond, "<=") {
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(cond, "<=")))
		if err == nil { return val <= n }
	}
	if strings.HasPrefix(cond, ">") {
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(cond, ">")))
		if err == nil { return val > n }
	}
	if strings.HasPrefix(cond, "<") {
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(cond, "<")))
		if err == nil { return val < n }
	}
	if strings.HasPrefix(cond, "==") {
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(cond, "==")))
		if err == nil { return val == n }
	}

	return false
}
