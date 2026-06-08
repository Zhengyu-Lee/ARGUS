package rules

func TestBoundaryBugFix(t *testing.T) {
	rulesContent := `
rules:
  - name: high-confidence
    when:
      confidence: ">= 80"
    then:
      publish_to: "ioc-confirmed"

  - name: medium-confidence
    when:
      confidence: "60-79"
    then:
      publish_to: "human-review"

  - name: low-confidence
    when:
      confidence: "< 60"
    then:
      publish_to: "es-store"
`
	path := writeTestRules(t, rulesContent)
	defer os.Remove(path)

	engine, err := NewEngine(path)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	tests := []struct {
		name       string
		confidence int
		wantTopic  string
	}{
		{"high 100", 100, "ioc-confirmed"},
		{"high 81", 81, "ioc-confirmed"},
		{"boundary 80 -> high (>=80)", 80, "ioc-confirmed"},
		{"mid 79 -> review", 79, "human-review"},
		{"mid 70", 70, "human-review"},
		{"mid 60", 60, "human-review"},
		{"low 59", 59, "es-store"},
		{"low 0", 0, "es-store"},
	}

	for _, tt := range tests {
		data := &types.EnrichedData{Confidence: tt.confidence}
		data.Content = "test"
		results := engine.Evaluate(data)

		found := false
		for _, r := range results {
			for _, topic := range r.TargetTopics {
				if topic == tt.wantTopic {
					found = true
				}
			}
		}

		if !found {
			t.Errorf("[%s] confidence=%d: expected to publish to %q, but didn't",
				tt.name, tt.confidence, tt.wantTopic)
		}
	}
}