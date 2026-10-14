package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiVersion = "2026-03-10"

type repository struct {
	ID          int64   `json:"id"`
	FullName    string  `json:"full_name"`
	Name        string  `json:"name"`
	HTMLURL     string  `json:"html_url"`
	URL         string  `json:"url"`
	Description *string `json:"description"`
	Stars       int     `json:"stargazers_count"`
	Watchers    int     `json:"watchers_count"`
	Forks       int     `json:"forks_count"`
	Issues      int     `json:"open_issues_count"`
	Size        int     `json:"size"`
	Language    string  `json:"language"`
	License     *struct {
		Name string `json:"name"`
		SPDX string `json:"spdx_id"`
	} `json:"license"`
	Topics   []string `json:"topics"`
	Fork     bool     `json:"fork"`
	Archived bool     `json:"archived"`
	Private  bool     `json:"private"`
	Branch   string   `json:"default_branch"`
	Created  string   `json:"created_at"`
	Updated  string   `json:"updated_at"`
	Pushed   string   `json:"pushed_at"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	StarredAt string `json:"starred_at"`
}
type starItem struct {
	StarredAt string     `json:"starred_at"`
	Repo      repository `json:"repo"`
}
type starWeek struct {
	Week  int64  `json:"week"`
	Total int    `json:"total"`
	Days  [7]int `json:"days"`
}
type ghCommit struct {
	SHA string `json:"sha"`
}
type readme struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}
type locResult struct {
	Loc        int                        `json:"loc"`
	LocByLangs map[string]int             `json:"locByLangs"`
	Children   map[string]json.RawMessage `json:"children"`
}
type fetchedRepo struct {
	repository
	Active         bool
	ActivityScore  int
	ActivityStatus string
	Languages      map[string]int
	Readme         string
	Commits        int
	Weeks          []starWeek
	StarStatus     string
	LOC            *locResult
	LOCStatus      string
	Error          string
}

func fetchAll(ctx context.Context, db *sql.DB, c *http.Client, token, base string) error {
	repos, err := listStarred(ctx, c, token, base)
	if err != nil {
		return fmt.Errorf("list starred repositories: %w", err)
	}
	now := time.Now().UTC()
	since := now.Add(-60 * 24 * time.Hour).Format(time.RFC3339)
	results := make([]fetchedRepo, 0, len(repos))
	for i, r := range repos {
		item := enrichRepo(ctx, c, token, base, since, r)
		results = append(results, item)
		fmt.Printf("fetched %d/%d %s\n", i+1, len(repos), r.FullName)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM repositories`); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM meta WHERE key IN ('fetch','tag')`); err != nil {
		return err
	}
	for _, x := range results {
		if _, err = insertRepo(tx, x); err != nil {
			return fmt.Errorf("save %s: %w", x.FullName, err)
		}
	}
	if err = setMetaTx(tx, "fetch", "ready"); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("saved %d repositories to SQLite\n", len(results))
	return nil
}

func enrichRepo(ctx context.Context, c *http.Client, token, base, since string, r repository) fetchedRepo {
	owner := r.Owner.Login
	if owner == "" {
		parts := strings.SplitN(r.FullName, "/", 2)
		if len(parts) > 1 {
			owner = parts[0]
		}
	}
	item := fetchedRepo{repository: r, StarStatus: "unavailable", LOCStatus: "unavailable"}
	item.Owner.Login = owner
	var err error
	item.Languages, err = getLanguages(ctx, c, token, base, owner, r.Name)
	if err != nil {
		item.Error = appendErr(item.Error, "languages: "+err.Error())
	}
	item.Commits, err = countCommits(ctx, c, token, base, owner, r.Name, since)
	if err != nil {
		item.Error = appendErr(item.Error, "commits: "+err.Error())
		item.ActivityStatus = "error"
	} else {
		item.ActivityStatus = "ready"
		item.Active = item.Commits >= 10
		item.ActivityScore = item.Commits
	}
	item.Weeks, err = getStarHistory(ctx, c, token, base, owner, r.Name)
	if err != nil {
		item.Error = appendErr(item.Error, "star history: "+err.Error())
	} else {
		item.StarStatus = "ready"
	}
	item.LOC, err = getLOC(ctx, c, owner, r.Name, r.Branch)
	if err != nil {
		item.Error = appendErr(item.Error, "LOC: "+err.Error())
	} else {
		item.LOCStatus = "ready"
	}
	item.Readme, err = getReadme(ctx, c, token, base, owner, r.Name)
	if err != nil && !isHTTPStatus(err, 404) {
		item.Error = appendErr(item.Error, "README: "+err.Error())
	}
	return item
}
func appendErr(a, b string) string {
	if a != "" {
		return a + "; " + b
	}
	return b
}
func listStarred(ctx context.Context, c *http.Client, token, base string) ([]repository, error) {
	u := strings.TrimRight(base, "/") + "/user/starred?per_page=100&sort=created&direction=desc"
	var out []repository
	for u != "" {
		var page []starItem
		next, e := githubGet(ctx, c, token, u, &page)
		if e != nil {
			return nil, e
		}
		for _, x := range page {
			x.Repo.StarredAt = x.StarredAt
			out = append(out, x.Repo)
		}
		u = next
	}
	return out, nil
}
func getLanguages(ctx context.Context, c *http.Client, token, base, owner, repo string) (map[string]int, error) {
	var m map[string]int
	_, e := githubGet(ctx, c, token, apiURL(base, "repos", owner, repo, "languages"), &m)
	if m == nil {
		m = map[string]int{}
	}
	return m, e
}
func countCommits(ctx context.Context, c *http.Client, token, base, owner, repo, since string) (int, error) {
	u := apiURL(base, "repos", owner, repo, "commits") + "?since=" + url.QueryEscape(since) + "&per_page=100"
	count := 0
	for u != "" {
		var page []ghCommit
		next, e := githubGet(ctx, c, token, u, &page)
		if e != nil {
			return 0, e
		}
		count += len(page)
		u = next
	}
	return count, nil
}
func getStarHistory(ctx context.Context, c *http.Client, token, base, owner, repo string) ([]starWeek, error) {
	baseURL := apiURL(base, "repos", owner, repo, "stargazers", "history")
	u := baseURL + "?per_page=30&page=1"
	out := []starWeek{}
	for p := 1; p <= 100 && u != ""; p++ {
		var page []starWeek
		next, err := githubGet(ctx, c, token, u, &page)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		out = append(out, page...)
		if p == 100 && len(page) == 30 {
			return nil, fmt.Errorf("star history exceeds GitHub API 100-page limit")
		}
		if next == "" {
			if len(page) < 30 {
				break
			}
			q, _ := url.Parse(baseURL)
			q.RawQuery = url.Values{"per_page": {"30"}, "page": {strconv.Itoa(p + 1)}}.Encode()
			next = q.String()
		}
		u = next
	}
	return out, nil
}
func getReadme(ctx context.Context, c *http.Client, token, base, owner, repo string) (string, error) {
	var r readme
	_, e := githubGet(ctx, c, token, apiURL(base, "repos", owner, repo, "readme"), &r)
	if e != nil {
		return "", e
	}
	if r.Encoding != "base64" {
		return "", fmt.Errorf("unsupported README encoding %q", r.Encoding)
	}
	b, e := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(r.Content), ""))
	if e != nil {
		return "", e
	}
	return firstWords(string(b), 1024), nil
}
func getLOC(ctx context.Context, c *http.Client, owner, repo, branch string) (*locResult, error) {
	raw := fmt.Sprintf("https://ghloc.ifels.dev/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	if branch != "" {
		raw += "/" + url.PathEscape(branch)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	q.Set("pretty", "false")
	u.RawQuery = q.Encode()
	req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", "application/json")
	resp, e := c.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if e != nil {
		return nil, e
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ghloc HTTP %s", resp.Status)
	}
	var out locResult
	if e = json.Unmarshal(b, &out); e != nil {
		return nil, e
	}
	return &out, nil
}
func githubGet(ctx context.Context, c *http.Client, token, raw string, dst any) (string, error) {
	for attempt := 0; attempt < 4; attempt++ {
		req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
		if e != nil {
			return "", e
		}
		accept := "application/vnd.github+json"
		if strings.Contains(raw, "/user/starred?") {
			accept = "application/vnd.github.star+json"
		}
		req.Header.Set("Accept", accept)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-GitHub-Api-Version", apiVersion)
		req.Header.Set("User-Agent", "StarBase/1.0")
		resp, e := c.Do(req)
		if e != nil {
			return "", e
		}
		b, re := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		next := resp.Header.Get("Link")
		status := resp.StatusCode
		retry := resp.Header.Get("Retry-After")
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		reset := resp.Header.Get("X-RateLimit-Reset")
		resp.Body.Close()
		if re != nil {
			return "", re
		}
		if status == http.StatusTooManyRequests || (status == http.StatusForbidden && (remaining == "0" || retry != "")) {
			delay := 30 * time.Second
			if n, e := strconv.Atoi(retry); e == nil && n >= 0 {
				delay = time.Duration(n) * time.Second
			} else if n, e := strconv.ParseInt(reset, 10, 64); e == nil {
				delay = time.Until(time.Unix(n, 0))
				if delay < time.Second {
					delay = time.Second
				}
			}
			if attempt == 3 {
				return "", fmt.Errorf("GitHub rate limit exhausted")
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if status < 200 || status >= 300 {
			return "", &httpStatusError{Status: status, Message: strings.TrimSpace(string(b))}
		}
		if e = json.Unmarshal(b, dst); e != nil {
			return "", e
		}
		return nextURL(next), nil
	}
	return "", fmt.Errorf("GitHub rate limit exhausted")
}

type httpStatusError struct {
	Status  int
	Message string
}

func (e *httpStatusError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message) }
func isHTTPStatus(e error, status int) bool {
	var h *httpStatusError
	return errorsAs(e, &h) && h.Status == status
}
func errorsAs(e error, target any) bool { return errors.As(e, target) }
func nextURL(link string) string {
	for _, p := range strings.Split(link, ",") {
		if strings.Contains(p, `rel="next"`) {
			a := strings.Index(p, "<")
			b := strings.Index(p, ">")
			if a >= 0 && b > a {
				return p[a+1 : b]
			}
		}
	}
	return ""
}
func apiURL(base string, parts ...string) string {
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.TrimRight(base, "/") + "/" + strings.Join(parts, "/")
}
func firstWords(s string, n int) string {
	fields := strings.Fields(s)
	if len(fields) > n {
		fields = fields[:n]
	}
	return strings.Join(fields, " ")
}
func setMetaTx(tx *sql.Tx, key, value string) error {
	_, e := tx.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return e
}
func insertRepo(tx *sql.Tx, x fetchedRepo) (int64, error) {
	desc := ""
	if x.Description != nil {
		desc = *x.Description
	}
	lic, spdx := "", ""
	if x.License != nil {
		lic = x.License.Name
		spdx = x.License.SPDX
	}
	langs, _ := json.Marshal(x.Languages)
	topics, _ := json.Marshal(x.Topics)
	weeks, _ := json.Marshal(x.Weeks)
	var total any
	byLang, tree := []byte(`{}`), []byte(`{}`)
	if x.LOC != nil {
		total = x.LOC.Loc
		byLang, _ = json.Marshal(x.LOC.LocByLangs)
		tree, _ = json.Marshal(x.LOC.Children)
	}
	result, e := tx.Exec(`INSERT INTO repositories(github_id,full_name,owner,name,description,html_url,api_url,stars,watchers,forks,open_issues,size_kb,primary_language,languages_json,license_name,license_spdx,topics_json,is_fork,is_archived,is_private,default_branch,created_at,updated_at,pushed_at,starred_at,readme_excerpt,activity_score,is_active,activity_status,commits_60d,star_history_json,star_history_status,loc_total,loc_by_language_json,loc_tree_json,loc_status,fetch_error,fetched_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, x.ID, x.FullName, x.Owner.Login, x.Name, desc, x.HTMLURL, x.URL, x.Stars, x.Watchers, x.Forks, x.Issues, x.Size, x.Language, string(langs), lic, spdx, string(topics), x.Fork, x.Archived, x.Private, x.Branch, x.Created, x.Updated, x.Pushed, x.StarredAt, x.Readme, x.Commits, x.Active, x.ActivityStatus, x.Commits, string(weeks), x.StarStatus, total, string(byLang), string(tree), x.LOCStatus, x.Error, time.Now().UTC().Format(time.RFC3339))
	if e != nil {
		return 0, e
	}
	id, e := result.LastInsertId()
	return id, e
}
