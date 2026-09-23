package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 4 << 20

type jevState struct {
	Task   string  `json:"task"`
	Skills []Skill `json:"skills"`
}

type jevQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type jevRequest struct {
	State     jevState               `json:"state"`
	Model     string                 `json:"model"`
	Questions map[string]jevQuestion `json:"questions"`
}

type jevAnswer struct {
	Type string   `json:"type"`
	Noul *float64 `json:"noul"`
}

type jevResponse struct {
	Model   string               `json:"model"`
	Answers map[string]jevAnswer `json:"answers"`
}

type batch struct {
	Request jevRequest
	Names   map[string]string
	Bytes   int
}

func newRequest(prompt, model string, skills []Skill) (jevRequest, map[string]string) {
	questions := make(map[string]jevQuestion, len(skills))
	names := make(map[string]string, len(skills))
	for i, skill := range skills {
		id := fmt.Sprintf("skill_%03d", i)
		questions[id] = jevQuestion{
			Type: "noul",
			Instructions: fmt.Sprintf(
				"Is the skill in `skills[%d]` directly relevant to performing the task in `task`? Judge its frontmatter description and scope, not merely keyword overlap.", i,
			),
			Criteria: map[string]string{
				"true":  "This skill would materially help perform the task.",
				"false": "This skill is unrelated or only superficially related to the task.",
			},
		}
		names[id] = skill.Name
	}
	return jevRequest{
		State:     jevState{Task: prompt, Skills: skills},
		Model:     model,
		Questions: questions,
	}, names
}

func makeBatches(prompt, model string, skills []Skill, maxSkills, maxBytes int) ([]batch, error) {
	var batches []batch
	for start := 0; start < len(skills); {
		end := start
		var accepted batch
		for end < len(skills) && end-start < maxSkills {
			request, names := newRequest(prompt, model, skills[start:end+1])
			body, err := json.Marshal(request)
			if err != nil {
				return nil, fmt.Errorf("encode Jev request: %w", err)
			}
			if len(body) > maxBytes {
				break
			}
			accepted = batch{Request: request, Names: names, Bytes: len(body)}
			end++
		}
		if end == start {
			request, _ := newRequest(prompt, model, skills[start:start+1])
			body, _ := json.Marshal(request)
			return nil, fmt.Errorf(
				"skill %q cannot fit in --batch-max-bytes=%d (serialized request is %d bytes); increase the limit or shorten its frontmatter/prompt",
				skills[start].Name, maxBytes, len(body),
			)
		}
		batches = append(batches, accepted)
		start = end
	}
	return batches, nil
}

type jevClient struct {
	Endpoint   string
	APIKey     string
	HTTPClient *http.Client
	MaxRetries int
}

func (c *jevClient) evaluate(ctx context.Context, request jevRequest) (jevResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return jevResponse{}, fmt.Errorf("encode Jev request: %w", err)
	}

	for attempt := 0; ; attempt++ {
		response, retryAfter, retryable, err := c.do(ctx, body)
		if err == nil {
			return response, nil
		}
		if !retryable || attempt >= c.MaxRetries {
			return jevResponse{}, err
		}
		delay := retryAfter
		if delay <= 0 {
			delay = time.Duration(1<<attempt) * 250 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return jevResponse{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *jevClient) do(ctx context.Context, body []byte) (jevResponse, time.Duration, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return jevResponse{}, 0, false, fmt.Errorf("create Jev request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return jevResponse{}, 0, false, fmt.Errorf("call Jev: %w", err)
		}
		return jevResponse{}, 0, true, fmt.Errorf("call Jev: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return jevResponse{}, 0, true, fmt.Errorf("read Jev response: %w", err)
	}
	if len(responseBody) > maxResponseBytes {
		return jevResponse{}, 0, false, fmt.Errorf("Jev response exceeds %d bytes", maxResponseBytes)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(responseBody))
		if len(message) > 1000 {
			message = message[:1000] + "..."
		}
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return jevResponse{}, parseRetryAfter(resp.Header.Get("Retry-After")), retryable,
			fmt.Errorf("Jev API returned %s: %s", resp.Status, message)
	}

	var decoded jevResponse
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	if err := decoder.Decode(&decoded); err != nil {
		return jevResponse{}, 0, false, fmt.Errorf("decode Jev response: %w", err)
	}
	if decoded.Answers == nil {
		return jevResponse{}, 0, false, fmt.Errorf("Jev response is missing answers")
	}
	return decoded, 0, false, nil
}

func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}
