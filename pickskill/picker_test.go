package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMakeBatchesHonorsCountAndByteLimits(t *testing.T) {
	skills := []Skill{
		{Name: "a", Frontmatter: "description: " + strings.Repeat("a", 100)},
		{Name: "b", Frontmatter: "description: " + strings.Repeat("b", 100)},
		{Name: "c", Frontmatter: "description: " + strings.Repeat("c", 100)},
	}
	one, _ := newRequest("task", "jev-latest", skills[:1])
	oneBody, _ := json.Marshal(one)
	two, _ := newRequest("task", "jev-latest", skills[:2])
	twoBody, _ := json.Marshal(two)
	limit := len(twoBody) - 1
	if limit <= len(oneBody) {
		t.Fatal("bad test setup")
	}

	batches, err := makeBatches("task", "jev-latest", skills, 2, limit)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 3 {
		t.Fatalf("got %d batches, want 3", len(batches))
	}
	for _, batch := range batches {
		if len(batch.Names) != 1 || batch.Bytes > limit {
			t.Fatalf("unexpected batch: names=%d bytes=%d limit=%d", len(batch.Names), batch.Bytes, limit)
		}
	}
}

func TestMakeBatchesRejectsOversizedSingleSkill(t *testing.T) {
	_, err := makeBatches("task", "jev-latest", []Skill{{Name: "huge", Frontmatter: strings.Repeat("x", 1000)}}, 10, 10)
	if err == nil || !strings.Contains(err.Error(), "huge") {
		t.Fatalf("got %v, want oversized skill error", err)
	}
}

func TestPickSkillsFiltersSortsCapsAndSendsExpectedShape(t *testing.T) {
	probabilities := map[string]float64{"alpha": 0.7, "beta": 0.9, "gamma": 0.2}
	var mu sync.Mutex
	var seen []jevRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		var request jevRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		seen = append(seen, request)
		mu.Unlock()
		answers := make(map[string]jevAnswer)
		for id, question := range request.Questions {
			if question.Type != "noul" {
				t.Errorf("question %s has type %q", id, question.Type)
			}
			index := 0
			if _, err := fmtSscanfIndex(question.Instructions, &index); err != nil {
				t.Error(err)
			}
			value := probabilities[request.State.Skills[index].Name]
			answers[id] = jevAnswer{Type: "noul", Noul: &value}
		}
		_ = json.NewEncoder(w).Encode(jevResponse{Model: "jev-test", Answers: answers})
	}))
	defer server.Close()

	client := &jevClient{Endpoint: server.URL, APIKey: "secret", HTTPClient: server.Client(), MaxRetries: 0}
	skills := []Skill{
		{Name: "alpha", Frontmatter: "description: alpha"},
		{Name: "beta", Frontmatter: "description: beta"},
		{Name: "gamma", Frontmatter: "description: gamma"},
	}
	got, err := pickSkills(context.Background(), client, "do a thing", "jev-latest", skills, 0.5, 1, 2, 100000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []scoredSkill{{Name: "beta", Probability: 0.9}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if len(seen) != 2 {
		t.Fatalf("got %d requests, want 2", len(seen))
	}
	for _, request := range seen {
		if request.State.Task != "do a thing" {
			t.Fatalf("unexpected task state: %q", request.State.Task)
		}
		for _, skill := range request.State.Skills {
			if skill.Frontmatter == "" {
				t.Fatal("frontmatter was not sent in state")
			}
		}
	}
}

func fmtSscanfIndex(instructions string, index *int) (string, error) {
	const prefix = "Is the skill in `skills["
	if !strings.HasPrefix(instructions, prefix) {
		return "", &parseError{instructions}
	}
	rest := strings.TrimPrefix(instructions, prefix)
	closing := strings.Index(rest, "]`")
	if closing < 0 {
		return "", &parseError{instructions}
	}
	var value int
	for _, r := range rest[:closing] {
		if r < '0' || r > '9' {
			return "", &parseError{instructions}
		}
		value = value*10 + int(r-'0')
	}
	*index = value
	return rest[closing+2:], nil
}

type parseError struct{ value string }

func (e *parseError) Error() string { return "cannot parse skill index from " + e.value }

func TestJevClientDoesNotRetryBadRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, `{"error":"bad input"}`, http.StatusBadRequest)
	}))
	defer server.Close()
	client := &jevClient{Endpoint: server.URL, APIKey: "secret", HTTPClient: &http.Client{Timeout: time.Second}, MaxRetries: 3}
	request, _ := newRequest("task", "jev-latest", []Skill{{Name: "a", Frontmatter: "description: a"}})
	_, err := client.evaluate(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "400 Bad Request") {
		t.Fatalf("got %v, want 400 error", err)
	}
	if calls != 1 {
		t.Fatalf("got %d calls, want 1", calls)
	}
}

func TestPickSkillsRejectsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"jev-test","answers":{}}`))
	}))
	defer server.Close()
	client := &jevClient{Endpoint: server.URL, APIKey: "secret", HTTPClient: server.Client()}
	_, err := pickSkills(context.Background(), client, "task", "jev-latest", []Skill{{Name: "a", Frontmatter: "description: a"}}, 0.5, 5, 10, 10000, nil)
	if err == nil || !strings.Contains(err.Error(), "missing answer") {
		t.Fatalf("got %v, want missing answer error", err)
	}
}
