package prow

import (
	"fmt"

	"github.com/Huang-Wei/25-kubecon-jp-codegen/pkg/git"
	"github.com/go-logr/logr"
	"sigs.k8s.io/prow/pkg/github"
)

func (p *Plugin) handleIssueComment(l logr.Logger, ic github.IssueCommentEvent) error {
	// Only consider new comments in PRs.
	if !ic.Issue.IsPullRequest() || ic.Action != github.IssueCommentActionCreated {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	num := ic.Issue.Number

	// Do not create a new logger, its fields are re-used by the caller in case of errors
	l = l.WithValues(
		github.OrgLogField, org,
		github.RepoLogField, repo,
		github.PrLogField, num,
	)

	// If the command is /deploy-dryrun, create a downstream PR and close it immediately.
	if codegenDryrunRe.MatchString(ic.Comment.Body) {
		pr, err := p.gitWorker.GetPullRequest(org, repo, num)
		if err != nil {
			return err
		}
		if pr.MergeSHA == nil {
			return nil
		}
		upstreamRepo := &git.GitHubRepo{
			Org:               org,
			Name:              repo,
			PullRequestNumber: num,
			MergeSHA:          *pr.MergeSHA,
		}
		// Create a dry-run PR.
		return p.createPullRequest(upstreamRepo, git.NewDryrunPRModifier())
	}

	if !codegenRe.MatchString(ic.Comment.Body) {
		return nil
	}
	l.Info("🚀 Requested a downstream codegen.")

	// Add the label and let PR handler process the codegen request.
	if err := p.gitWorker.AddLabel(org, repo, num, CodegenLabel); err != nil {
		return fmt.Errorf("failed to add label %q: %w", CodegenLabel, err)
	}
	return nil
}
