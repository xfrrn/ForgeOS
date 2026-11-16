package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"forgeos/internal/store"
)

func pollComments(ctx context.Context, data pollStore, get githubGET, taskID int64, issue store.Issue) error {
	var previousLast int64
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := fmt.Sprintf("repositories/%d/issues/%d/comments?per_page=%d&page=%d", issue.RepositoryID, issue.Number, pageSize, page)
		var entries []json.RawMessage
		if err := get(ctx, path, &entries); err != nil {
			return err
		}
		if len(entries) > pageSize {
			return errGHRead
		}
		var last int64
		for _, entry := range entries {
			var c struct {
				ID        int64
				Body      *string
				IssueURL  string    `json:"issue_url"`
				HTMLURL   string    `json:"html_url"`
				UpdatedAt time.Time `json:"updated_at"`
			}
			if json.Unmarshal(entry, &c) != nil || c.ID <= 0 || c.UpdatedAt.IsZero() || c.Body == nil || len(*c.Body) > 256<<10 || strings.ContainsRune(*c.Body, 0) ||
				c.IssueURL != fmt.Sprintf("https://api.github.com/repos/%s/issues/%d", issue.Repository, issue.Number) || c.HTMLURL != fmt.Sprintf("%s#issuecomment-%d", issue.URL, c.ID) {
				return errIdentity
			}
			last = c.ID
		}
		if len(entries) > 0 {
			if len(entries) == pageSize && last <= previousLast {
				return errGHRead
			}
			if err := data.RecordComments(ctx, taskID, issue, entries); err != nil {
				return err
			}
		}
		if len(entries) < pageSize {
			return nil
		}
		previousLast = last
	}
}
