package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

const defaultEndpoint = "https://api.typesafe.ai/v1/systemone"

type options struct {
	skillsDir      string
	prompt         string
	threshold      float64
	maxSkills      int
	batchMaxSkills int
	batchMaxBytes  int
	model          string
	endpoint       string
	apiKeyEnv      string
	timeout        time.Duration
	retries        int
	verbose        bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pickskill:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("pickskill", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var opts options
	flags.StringVar(&opts.skillsDir, "skillsdir", "", "directory containing <skill-name>/SKILL.md entries (required)")
	flags.StringVar(&opts.prompt, "prompt", "", "task prompt used to select skills (required)")
	flags.Float64Var(&opts.threshold, "threshold", 0.5, "minimum Jev relevance probability in [0,1]")
	flags.IntVar(&opts.maxSkills, "max-skills", 5, "maximum skills to return; 0 means unlimited")
	flags.IntVar(&opts.batchMaxSkills, "batch-max-skills", 100, "maximum skills in one Jev request")
	flags.IntVar(&opts.batchMaxBytes, "batch-max-bytes", 24*1024, "maximum serialized JSON bytes in one Jev request")
	flags.StringVar(&opts.model, "model", "jev-latest", "Jev model ID or alias")
	flags.StringVar(&opts.endpoint, "endpoint", defaultEndpoint, "Jev System One endpoint")
	flags.StringVar(&opts.apiKeyEnv, "api-key-env", "TYPESAFE_API_KEY", "environment variable containing the Jev API key")
	flags.DurationVar(&opts.timeout, "timeout", 60*time.Second, "timeout for each HTTP request")
	flags.IntVar(&opts.retries, "retries", 2, "retries for transient network, 429, and 5xx failures")
	flags.BoolVar(&opts.verbose, "verbose", false, "print batch and score diagnostics to stderr")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: pickskill --skillsdir DIR --prompt TEXT [options]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	if err := validateOptions(opts); err != nil {
		return err
	}

	apiKey := os.Getenv(opts.apiKeyEnv)
	if apiKey == "" {
		return fmt.Errorf("environment variable %s is required", opts.apiKeyEnv)
	}
	skills, err := loadSkills(opts.skillsDir)
	if err != nil {
		return err
	}
	client := &jevClient{
		Endpoint:   opts.endpoint,
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: opts.timeout},
		MaxRetries: opts.retries,
	}
	var progress func(int, int, int, int)
	if opts.verbose {
		fmt.Fprintf(os.Stderr, "loaded %d skills from %s\n", len(skills), opts.skillsDir)
		progress = func(index, total, count, bytes int) {
			fmt.Fprintf(os.Stderr, "evaluating batch %d/%d (%d skills, %d bytes)\n", index, total, count, bytes)
		}
	}
	scored, err := pickSkills(
		context.Background(), client, opts.prompt, opts.model, skills, opts.threshold,
		opts.maxSkills, opts.batchMaxSkills, opts.batchMaxBytes, progress,
	)
	if err != nil {
		return err
	}
	names := make([]string, len(scored))
	for i, skill := range scored {
		names[i] = skill.Name
		if opts.verbose {
			fmt.Fprintf(os.Stderr, "%.6f\t%s\n", skill.Probability, skill.Name)
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(names)
}

func validateOptions(opts options) error {
	if opts.skillsDir == "" {
		return fmt.Errorf("--skillsdir is required")
	}
	if opts.prompt == "" {
		return fmt.Errorf("--prompt is required")
	}
	if opts.threshold < 0 || opts.threshold > 1 {
		return fmt.Errorf("--threshold must be between 0 and 1")
	}
	if opts.maxSkills < 0 {
		return fmt.Errorf("--max-skills must be at least 0")
	}
	if opts.batchMaxSkills < 1 {
		return fmt.Errorf("--batch-max-skills must be at least 1")
	}
	if opts.batchMaxBytes < 1 {
		return fmt.Errorf("--batch-max-bytes must be at least 1")
	}
	if opts.model == "" || opts.endpoint == "" || opts.apiKeyEnv == "" {
		return fmt.Errorf("--model, --endpoint, and --api-key-env cannot be empty")
	}
	if opts.timeout <= 0 {
		return fmt.Errorf("--timeout must be greater than zero")
	}
	if opts.retries < 0 {
		return fmt.Errorf("--retries must be at least 0")
	}
	return nil
}
