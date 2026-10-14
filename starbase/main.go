package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	_ "modernc.org/sqlite"
	"net/http"
	"os"
	"time"
)

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS repositories (
 id INTEGER PRIMARY KEY, github_id INTEGER NOT NULL DEFAULT 0, full_name TEXT NOT NULL UNIQUE, owner TEXT NOT NULL, name TEXT NOT NULL,
 description TEXT NOT NULL DEFAULT '', html_url TEXT NOT NULL, api_url TEXT NOT NULL,
 stars INTEGER NOT NULL DEFAULT 0, watchers INTEGER NOT NULL DEFAULT 0, forks INTEGER NOT NULL DEFAULT 0,
 open_issues INTEGER NOT NULL DEFAULT 0, size_kb INTEGER NOT NULL DEFAULT 0, primary_language TEXT NOT NULL DEFAULT '',
 languages_json TEXT NOT NULL DEFAULT '{}', license_name TEXT NOT NULL DEFAULT '', license_spdx TEXT NOT NULL DEFAULT '',
 topics_json TEXT NOT NULL DEFAULT '[]', is_fork INTEGER NOT NULL DEFAULT 0, is_archived INTEGER NOT NULL DEFAULT 0,
 is_private INTEGER NOT NULL DEFAULT 0, default_branch TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', pushed_at TEXT NOT NULL DEFAULT '', starred_at TEXT NOT NULL DEFAULT '',
 readme_excerpt TEXT NOT NULL DEFAULT '', activity_score INTEGER NOT NULL DEFAULT 0, is_active INTEGER NOT NULL DEFAULT 0,
 activity_status TEXT NOT NULL DEFAULT 'pending', commits_60d INTEGER NOT NULL DEFAULT 0,
 star_history_json TEXT NOT NULL DEFAULT '[]', star_history_status TEXT NOT NULL DEFAULT 'pending',
 loc_total INTEGER, loc_by_language_json TEXT NOT NULL DEFAULT '{}', loc_tree_json TEXT NOT NULL DEFAULT '{}', loc_status TEXT NOT NULL DEFAULT 'pending',
 tag_status TEXT NOT NULL DEFAULT 'pending', fetch_error TEXT NOT NULL DEFAULT '', fetched_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS repository_tags (
 repository_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
 tag TEXT NOT NULL, probability REAL NOT NULL, PRIMARY KEY(repository_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_repositories_stars ON repositories(stars DESC);
CREATE INDEX IF NOT EXISTS idx_repositories_activity ON repositories(activity_score DESC);
CREATE INDEX IF NOT EXISTS idx_repositories_loc ON repositories(loc_total DESC);
CREATE INDEX IF NOT EXISTS idx_repositories_starred ON repositories(starred_at DESC);
CREATE INDEX IF NOT EXISTS idx_repository_tags_tag ON repository_tags(tag);
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal("starbase: ", err)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: starbase fetch|tag|refresh|serve [flags]")
	}
	switch args[0] {
	case "fetch":
		return runFetch(args[1:])
	case "tag":
		return runTag(args[1:])
	case "refresh":
		return runRefresh(args[1:])
	case "serve":
		return runServe(args[1:])
	case "--help", "-h":
		fmt.Println("Usage: starbase fetch|tag|refresh|serve [flags]")
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err = ensureGitHubIDColumn(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureGitHubIDColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(repositories)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var defaultValue any
		if err = rows.Scan(&cid, &name, &kind, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "github_id" {
			found = true
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if !found {
		_, err = db.Exec(`ALTER TABLE repositories ADD COLUMN github_id INTEGER NOT NULL DEFAULT 0`)
	}
	return err
}
func ready(db *sql.DB, stage string) bool {
	var s string
	err := db.QueryRow(`SELECT value FROM meta WHERE key=?`, stage).Scan(&s)
	return err == nil && s == "ready"
}
func setMeta(db *sql.DB, key, value string) error {
	_, e := db.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return e
}
func runFetch(args []string) error {
	f := flag.NewFlagSet("fetch", flag.ContinueOnError)
	dbPath := f.String("db", "starbase.db", "SQLite database")
	tokenEnv := f.String("github-token-env", "GH_TOKEN", "GitHub token environment variable")
	api := f.String("github-api", "https://api.github.com", "GitHub REST API base URL")
	timeout := f.Duration("timeout", 30*time.Second, "HTTP timeout")
	if e := f.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if f.NArg() > 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	token := os.Getenv(*tokenEnv)
	if token == "" {
		return fmt.Errorf("environment variable %s is required", *tokenEnv)
	}
	d, e := openDB(*dbPath)
	if e != nil {
		return e
	}
	defer d.Close()
	ctx := context.Background()
	c := &http.Client{Timeout: *timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return fetchAll(ctx, d, c, token, *api)
}
func runTag(args []string) error {
	f := flag.NewFlagSet("tag", flag.ContinueOnError)
	dbPath := f.String("db", "starbase.db", "SQLite database")
	tagsFile := f.String("tagsfile", "tags.txt", "valid tags, one per line")
	endpoint := f.String("endpoint", "https://api.typesafe.ai/v1/systemone", "Jev endpoint")
	model := f.String("model", "jev-latest", "Jev model")
	keyEnv := f.String("api-key-env", "TYPESAFE_API_KEY", "Jev API key environment variable")
	timeout := f.Duration("timeout", 60*time.Second, "HTTP timeout")
	if e := f.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if f.NArg() > 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	key := os.Getenv(*keyEnv)
	if key == "" {
		return fmt.Errorf("environment variable %s is required", *keyEnv)
	}
	tags, e := readValidTags(*tagsFile)
	if e != nil {
		return e
	}
	d, e := openDB(*dbPath)
	if e != nil {
		return e
	}
	defer d.Close()
	if !ready(d, "fetch") {
		return fmt.Errorf("fetch must complete before tag")
	}
	client := &http.Client{Timeout: *timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return tagAll(context.Background(), d, client, *endpoint, key, *model, tags)
}

func runRefresh(args []string) error {
	f := flag.NewFlagSet("refresh", flag.ContinueOnError)
	dbPath := f.String("db", "starbase.db", "SQLite database")
	githubTokenEnv := f.String("github-token-env", "GH_TOKEN", "GitHub token environment variable")
	githubAPI := f.String("github-api", "https://api.github.com", "GitHub REST API base URL")
	jevKeyEnv := f.String("api-key-env", "TYPESAFE_API_KEY", "Jev API key environment variable")
	jevEndpoint := f.String("endpoint", "https://api.typesafe.ai/v1/systemone", "Jev endpoint")
	jevModel := f.String("model", "jev-latest", "Jev model")
	tagsFile := f.String("tagsfile", "tags.txt", "valid tags, one per line")
	timeout := f.Duration("timeout", 60*time.Second, "HTTP timeout")
	if e := f.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if f.NArg() > 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	githubToken := os.Getenv(*githubTokenEnv)
	if githubToken == "" {
		return fmt.Errorf("environment variable %s is required", *githubTokenEnv)
	}
	db, err := openDB(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if !ready(db, "fetch") || !ready(db, "tag") {
		return fmt.Errorf("fetch and tag must complete before refresh")
	}
	client := &http.Client{Timeout: *timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	jevKey := os.Getenv(*jevKeyEnv)
	return refreshAll(context.Background(), db, client, githubToken, *githubAPI, client, *jevEndpoint, jevKey, *jevModel, *tagsFile)
}

func runServe(args []string) error {
	f := flag.NewFlagSet("serve", flag.ContinueOnError)
	dbPath := f.String("db", "starbase.db", "SQLite database")
	addr := f.String("addr", "127.0.0.1:8080", "HTTP listen address")
	pageSize := f.Int("page-size", 25, "repositories per page")
	if e := f.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if f.NArg() > 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *pageSize < 1 || *pageSize > 200 {
		return fmt.Errorf("page-size must be from 1 to 200")
	}
	d, e := openDB(*dbPath)
	if e != nil {
		return e
	}
	defer d.Close()
	if !ready(d, "fetch") || !ready(d, "tag") {
		return fmt.Errorf("fetch and tag must both complete before serve")
	}
	return serve(d, *addr, *pageSize)
}
