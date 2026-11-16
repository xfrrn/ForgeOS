package acceptance

import (
	"context"

	"forgeos/internal/localrepo"
	"forgeos/internal/validation"
	"forgeos/schemas"
)

type Source struct {
	Basis     Basis                   `json:"basis"`
	Library   schemas.PlaybookLibrary `json:"library"`
	Diff      string                  `json:"diff"`
	Truncated bool                    `json:"diff_truncated"`
}

func ReadSource(ctx context.Context, root, project, base, commit string, policy validation.Policy, criteria []string) (Source, error) {
	var src Source
	repo, err := localrepo.ResolveRepo(ctx, root, project)
	if err != nil {
		return src, err
	}
	d, err := validation.Assess(ctx, repo, base, commit, policy)
	if err != nil {
		return src, err
	}
	src.Library, err = schemas.GlobalPlaybooks()
	if err != nil {
		return src, err
	}
	src.Basis, err = NewBasis(d, src.Library, criteria)
	if err != nil {
		return src, err
	}
	diff, truncated, err := localrepo.LimitedGit(ctx, repo, 128<<10, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--no-color", base, commit, "--")
	src.Diff, src.Truncated = string(diff), truncated
	return src, err
}
