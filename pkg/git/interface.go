package git

import (
	"context"

	"github.com/go-logr/logr"
)

const (
	KubeConCodegenRepoName = "Huang-Wei/25-kubecon-jp-codegen"
)

type CodegenFunc func(ctx context.Context, dstDir string, prune bool) error

type Worker interface {
	// CreatePullRequestAllTenants creates a pull request covering changes to all tenants defined in UpstreamRepo.
	CreatePullRequestAllTenants(context.Context, CodegenFunc) error
	// AddLabel adds the given 'label' to the 'org/repo' repo.
	AddLabel(org, repo string, number int, label string) error
	// Logger returns the underlying logger for the worker.
	Logger() logr.Logger
}
