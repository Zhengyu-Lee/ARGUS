package types

import "time"

type RawData struct {
	ID        string            `json:"id"`
	Source    string            `json:"source"`    // rss, device-farm, api
	Platform  string            `json:"platform"`  // weibo, douyin, xiaohongshu, xianyu...
	URL       string            `json:"url"`
	Title     string            `json:"title"`
	Content   string            `json:"content"`
	Author    string            `json:"author"`
	Published time.Time         `json:"published"`
	Collected time.Time         `json:"collected"`
	Metadata  map[string]string `json:"metadata"`
}

type EnrichedData struct {
	RawData
	Entities   []Entity   `json:"entities"`
	Tags       []string   `json:"tags"`
	Confidence int        `json:"confidence"` // 0-100
	IOCs       []IOC      `json:"iocs"`
	Sentiment  string     `json:"sentiment"` // positive, negative, neutral
	Category   string     `json:"category"`  // vulnerability, phishing, leak, public-opinion
}

type Entity struct {
	Type  string `json:"type"`  // person, org, location, event
	Name  string `json:"name"`
	Value string `json:"value"`
}

type IOC struct {
	Type  string `json:"type"`  // ip, domain, url, hash, cve, email
	Value string `json:"value"`
}

type RuleResult struct {
	Matched        bool     `json:"matched"`
	RuleName       string   `json:"rule_name"`
	ConfidenceBoost int     `json:"confidence_boost"`
	TargetTopics   []string `json:"target_topics"` // kafka topics to publish to
	TriggerAlert   bool     `json:"trigger_alert"`
	NeedsReview    bool     `json:"needs_review"`
}
