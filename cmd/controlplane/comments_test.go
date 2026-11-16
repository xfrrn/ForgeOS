package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"forgeos/internal/store"
)

func TestCommentPagesPreserveSourceAndRejectWrongIssue(t *testing.T) {
	data := &pollFixture{}
	issue := store.Issue{RepositoryID: 41, Number: 2, Repository: "org/project", URL: "https://github.com/org/project/issues/2"}
	get := func(_ context.Context, path string, dest any) error {
		n, first := pageSize, 1
		switch path {
		case "repositories/41/issues/2/comments?per_page=30&page=1":
		case "repositories/41/issues/2/comments?per_page=30&page=2":
			n, first = 1, 31
		default:
			t.Fatal(path)
		}
		for i := first; i < first+n; i++ {
			*dest.(*[]json.RawMessage) = append(*dest.(*[]json.RawMessage), json.RawMessage(fmt.Sprintf(
				`{"id":%d,"issue_url":"https://api.github.com/repos/org/project/issues/2","html_url":"https://github.com/org/project/issues/2#issuecomment-%d","body":"note","updated_at":"2026-01-01T00:00:00Z","extra":{"retained":true}}`, i, i)))
		}
		return nil
	}
	if err := pollComments(context.Background(), data, get, 7, issue); err != nil || len(data.comments) != 31 {
		t.Fatal(len(data.comments), err)
	}
	issue.Repository = "another/project"
	if err := pollComments(context.Background(), data, get, 7, issue); !errors.Is(err, errIdentity) || len(data.comments) != 31 {
		t.Fatal("accepted unrelated comment", err)
	}
}
