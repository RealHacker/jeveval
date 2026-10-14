package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type storedRepo struct {
	id       int64
	githubID int64
	fullName string
}

type pendingRepo struct {
	repo fetchedRepo
	tags []tagProbability
}

func refreshAll(ctx context.Context, db *sql.DB, githubClient *http.Client, githubToken, githubAPI string, jevClient *http.Client, jevEndpoint, jevKey, jevModel, tagsFile string) error {
	starred, err := listStarred(ctx, githubClient, githubToken, githubAPI)
	if err != nil {
		return fmt.Errorf("list starred repositories: %w", err)
	}
	rows, err := db.Query(`SELECT id,github_id,full_name FROM repositories`)
	if err != nil {
		return err
	}
	var current []storedRepo
	for rows.Next() {
		var repo storedRepo
		if err = rows.Scan(&repo.id, &repo.githubID, &repo.fullName); err != nil {
			rows.Close()
			return err
		}
		current = append(current, repo)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}

	byGitHubID := make(map[int64]storedRepo, len(current))
	byName := make(map[string]storedRepo, len(current))
	for _, repo := range current {
		byName[strings.ToLower(repo.fullName)] = repo
		if repo.githubID != 0 {
			byGitHubID[repo.githubID] = repo
		}
	}
	matched := make(map[int64]bool, len(current))
	legacyIDs := make(map[int64]int64)
	var added []repository
	seenGitHubID := make(map[int64]bool, len(starred))
	for _, repo := range starred {
		if repo.ID != 0 {
			if seenGitHubID[repo.ID] {
				continue
			}
			seenGitHubID[repo.ID] = true
		}
		if existing, ok := byGitHubID[repo.ID]; ok && repo.ID != 0 {
			matched[existing.id] = true
			continue
		}
		if existing, ok := byName[strings.ToLower(repo.FullName)]; ok {
			matched[existing.id] = true
			if existing.githubID == 0 && repo.ID != 0 {
				legacyIDs[existing.id] = repo.ID
			}
			continue
		}
		added = append(added, repo)
	}
	var deleted []int64
	for _, repo := range current {
		if !matched[repo.id] {
			deleted = append(deleted, repo.id)
		}
	}
	if len(added) == 0 && len(deleted) == 0 && len(legacyIDs) == 0 {
		fmt.Printf("refresh complete: no changes across %d repositories\n", len(current))
		return nil
	}

	since := time.Now().UTC().Add(-60 * 24 * time.Hour).Format(time.RFC3339)
	pending := make([]pendingRepo, 0, len(added))
	for i, repo := range added {
		fetched := enrichRepo(ctx, githubClient, githubToken, githubAPI, since, repo)
		pending = append(pending, pendingRepo{repo: fetched})
		fmt.Printf("fetched new repo %d/%d %s\n", i+1, len(added), repo.FullName)
	}
	if len(pending) > 0 {
		if jevKey == "" {
			return fmt.Errorf("environment variable for the Jev API key is required when refresh finds new repositories")
		}
		tags, err := readValidTags(tagsFile)
		if err != nil {
			return err
		}
		for i := range pending {
			pending[i].tags, err = evaluateRepoTags(ctx, jevClient, jevEndpoint, jevKey, jevModel, pending[i].repo, tags)
			if err != nil {
				return fmt.Errorf("tag new repository %s: %w", pending[i].repo.FullName, err)
			}
			fmt.Printf("tagged new repo %d/%d %s (%d tags)\n", i+1, len(pending), pending[i].repo.FullName, len(pending[i].tags))
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range deleted {
		if _, err = tx.Exec(`DELETE FROM repository_tags WHERE repository_id=?`, id); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM repositories WHERE id=?`, id); err != nil {
			return err
		}
	}
	for id, githubID := range legacyIDs {
		if _, err = tx.Exec(`UPDATE repositories SET github_id=? WHERE id=?`, githubID, id); err != nil {
			return err
		}
	}
	for _, item := range pending {
		repoID, insertErr := insertRepo(tx, item.repo)
		if insertErr != nil {
			return fmt.Errorf("save %s: %w", item.repo.FullName, insertErr)
		}
		for _, tag := range item.tags {
			if _, err = tx.Exec(`INSERT INTO repository_tags(repository_id,tag,probability) VALUES(?,?,?)`, repoID, tag.Name, tag.Probability); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(`UPDATE repositories SET tag_status='ready' WHERE id=?`, repoID); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("refresh complete: added %d, removed %d, kept %d repositories\n", len(added), len(deleted), len(current)-len(deleted))
	return nil
}
