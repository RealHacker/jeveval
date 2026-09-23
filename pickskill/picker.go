package main

import (
	"context"
	"fmt"
	"sort"
)

type scoredSkill struct {
	Name        string
	Probability float64
}

func pickSkills(
	ctx context.Context,
	client *jevClient,
	prompt, model string,
	skills []Skill,
	threshold float64,
	maxResults, batchMaxSkills, batchMaxBytes int,
	progress func(index, total, count, bytes int),
) ([]scoredSkill, error) {
	batches, err := makeBatches(prompt, model, skills, batchMaxSkills, batchMaxBytes)
	if err != nil {
		return nil, err
	}

	var scored []scoredSkill
	for i, current := range batches {
		if progress != nil {
			progress(i+1, len(batches), len(current.Names), current.Bytes)
		}
		response, err := client.evaluate(ctx, current.Request)
		if err != nil {
			return nil, fmt.Errorf("evaluate batch %d/%d: %w", i+1, len(batches), err)
		}
		for id, name := range current.Names {
			answer, ok := response.Answers[id]
			if !ok {
				return nil, fmt.Errorf("batch %d/%d: Jev response is missing answer %q", i+1, len(batches), id)
			}
			if answer.Type != "noul" || answer.Noul == nil {
				return nil, fmt.Errorf("batch %d/%d: answer %q is not a noul answer", i+1, len(batches), id)
			}
			if *answer.Noul < 0 || *answer.Noul > 1 {
				return nil, fmt.Errorf("batch %d/%d: answer %q has probability %v outside [0,1]", i+1, len(batches), id, *answer.Noul)
			}
			if *answer.Noul >= threshold {
				scored = append(scored, scoredSkill{Name: name, Probability: *answer.Noul})
			}
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Probability == scored[j].Probability {
			return scored[i].Name < scored[j].Name
		}
		return scored[i].Probability > scored[j].Probability
	})
	if maxResults > 0 && len(scored) > maxResults {
		scored = scored[:maxResults]
	}
	return scored, nil
}
