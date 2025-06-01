package prow

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sync"

	"github.com/go-logr/logr"
	"sigs.k8s.io/prow/pkg/config"
	"sigs.k8s.io/prow/pkg/github"
	"sigs.k8s.io/prow/pkg/pluginhelp"

	"github.com/Huang-Wei/25-kubecon-jp-codegen/pkg/git"
)

const (
	PluginName   = "kubecon-codegen"
	codegenLabel = "post-merge/codegen"
)

var (
	codegenRe = regexp.MustCompile(`(?mi)^/codegen\s*$`)
)

var _ http.Handler = &Plugin{}

// Plugin implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Plugin struct {
	sync.Mutex

	tokenGenerator func() []byte
	gitWorker      git.Worker

	logger logr.Logger
}

func NewPlugin(
	tokenGenerator func() []byte,
	gitWorker git.Worker,
) *Plugin {
	return &Plugin{
		tokenGenerator: tokenGenerator,
		gitWorker:      gitWorker,
		logger:         gitWorker.Logger(),
	}
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (p *Plugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok, _ := github.ValidateWebhook(w, r, p.tokenGenerator)
	if !ok {
		return
	}
	_, _ = fmt.Fprint(w, "Event received. Have a nice day.")

	if err := p.handleEvent(eventType, eventGUID, payload); err != nil {
		p.logger.Error(err, "error parsing event.")
	}
}

func (p *Plugin) handleEvent(eventType, eventGUID string, payload []byte) error {
	logger := p.logger.WithValues(
		"event-type",
		eventType,
		github.EventGUID,
		eventGUID,
	)
	switch eventType {
	case "issue_comment":
		var ic github.IssueCommentEvent
		if err := json.Unmarshal(payload, &ic); err != nil {
			return err
		}
		go func() {
			if err := p.handleIssueComment(logger, ic); err != nil {
				logger.Error(err, fmt.Sprintf("%v failed.", PluginName))
			}
		}()
	case "pull_request":
		var pr github.PullRequestEvent
		if err := json.Unmarshal(payload, &pr); err != nil {
			return err
		}
		go func() {
			if err := p.handlePullRequest(logger, pr); err != nil {
				logger.Error(err, fmt.Sprintf("%v failed.", PluginName))
			}
		}()
	default:
		logger.Info("skipping event")
	}
	return nil
}

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

	if !codegenRe.MatchString(ic.Comment.Body) {
		return nil
	}
	l.Info("🚀 Requested a downstream codegen.")

	// Add the label and let PR handler process the codegen request.
	if err := p.gitWorker.AddLabel(org, repo, num, codegenLabel); err != nil {
		return fmt.Errorf("failed to add label %q: %w", codegenLabel, err)
	}
	return nil
}

func (p *Plugin) handlePullRequest(_ logr.Logger, _ github.PullRequestEvent) error {
	return nil
}

// HelpProvider construct the pluginhelp.PluginHelp for this plugin.
func HelpProvider(_ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: fmt.Sprintf("The %s plugin is used to automatically create downstream PRs.", PluginName),
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/deploy",
		Description: "Create downstream PRs to deploy these changes",
		Featured:    true,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/deploy"},
	})
	return pluginHelp, nil
}
