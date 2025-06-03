package prow

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/prow/pkg/github"

	"github.com/Huang-Wei/25-kubecon-jp-codegen/pkg/generator"
	"github.com/Huang-Wei/25-kubecon-jp-codegen/pkg/git"
)

const (
	CodegenLabel           = "post-merge/codegen"
	KubeConCodegenRepoName = "Huang-Wei/25-kubecon-jp-codegen"
)

func (p *Plugin) handlePullRequest(l logr.Logger, pre github.PullRequestEvent) error {
	// Only consider newly merged PRs
	if pre.Action != github.PullRequestActionClosed && pre.Action != github.PullRequestActionLabeled {
		l.Info("ignoring event, not one of (closed, labeled)")
		return nil
	}

	l.Info(fmt.Sprintf("processing event for PR #%v", pre.Number))

	pr := pre.PullRequest
	if !pr.Merged || pr.MergeSHA == nil {
		l.Info("ignoring event, PR is not merged or does not have a merge commit")
		return nil
	}

	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name
	num := pr.Number
	mergeSHA := *pr.MergeSHA

	// Do not create a new logger, its fields are re-used by the caller in case of errors
	l = l.WithValues(
		github.OrgLogField, org,
		github.RepoLogField, repo,
		github.PrLogField, num,
	)

	hasLabel := false
	for _, label := range pr.Labels {
		if label.Name == CodegenLabel {
			hasLabel = true
			break
		}
	}

	if !hasLabel {
		l.Info(fmt.Sprintf("PR was merged or labels were changed, but label %q is not present", CodegenLabel))
		return nil
	}
	l.Info(fmt.Sprintf("PR is labeled with %q, proceeding with codegen", CodegenLabel))

	p.Lock()
	defer p.Unlock()

	upstreamRepo := &git.GitHubRepo{
		Org:               org,
		Name:              repo,
		PullRequestNumber: num,
		MergeSHA:          mergeSHA,
	}

	return p.createPullRequest(upstreamRepo, git.NewDeployPRModifier())
}

func (p *Plugin) createPullRequest(upstreamRepo *git.GitHubRepo, prModifier git.PullRequestModifier) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Pre-process the user input in the upstream repo.
	// TODO: work on the real codegen
	_, err := p.gitWorker.FetchUpstreamConfigs(ctx, upstreamRepo)
	if err != nil {
		return err
	}

	// Create a downstream codegen PR.
	cg := generator.NewCodegen()
	return p.gitWorker.CreatePullRequest(ctx, upstreamRepo, prModifier, KubeConCodegenRepoName, cg.FanOutArtifacts)
}
