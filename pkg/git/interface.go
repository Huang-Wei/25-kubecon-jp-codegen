package git

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/prow/pkg/github"

	"github.com/Huang-Wei/25-kubecon-jp-codegen/pkg/internal"
)

type CodegenFunc func(ctx context.Context, dstDir string) error

type Worker interface {
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	// CreatePullRequest creates a pull request covering changes to all infra input defined in UpstreamRepo.
	CreatePullRequest(context.Context, *GitHubRepo, PullRequestModifier, string, CodegenFunc) error
	// FetchUpstreamConfigs scans, parse and pre-process the given repo's user input into XYZTuple list.
	FetchUpstreamConfigs(ctx context.Context, repo *GitHubRepo) ([]*internal.TenantTuple, error)
	// AddLabel adds the given 'label' to the 'org/repo' repo.
	AddLabel(org, repo string, number int, label string) error
	// Logger returns the underlying logger for the worker.
	Logger() logr.Logger
}

// PullRequestModifier is an interface to tweak the generated pull request's behavior.
type PullRequestModifier interface {
	TitleTag() string
	PostCommentPrefix() string
	BranchPostFix() string
	NoopMsg(GitHubRepo) string
	TearDown() bool
}
