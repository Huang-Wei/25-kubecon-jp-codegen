package git

import "fmt"

var (
	noopMsgTemplate = "⏹️ %sNo changes on cloud resources detected. Skip creating downstream %s/%s PR"

	_ PullRequestModifier = DeployPRModifier{}
	_ PullRequestModifier = DryrunPRModifier{}
)

type DeployPRModifier struct{}

func (d DeployPRModifier) TearDown() bool {
	return false
}

func (d DeployPRModifier) NoopMsg(repo GHRepo) string {
	return fmt.Sprintf(noopMsgTemplate, "", repo.Org, repo.Name)
}

func (d DeployPRModifier) BranchPostFix() string {
	return ""
}

func (d DeployPRModifier) TitleTag() string {
	return "[Auto-generated] "
}

func (d DeployPRModifier) PostCommentPrefix() string {
	return "⭐ Auto-generated a PR: "
}

func NewDeployPRModifier() DeployPRModifier {
	return DeployPRModifier{}
}

type DryrunPRModifier struct{}

func (d DryrunPRModifier) TearDown() bool {
	return true
}

func (d DryrunPRModifier) NoopMsg(repo GHRepo) string {
	return fmt.Sprintf(noopMsgTemplate, "[DRYRUN] ", repo.Org, repo.Name)
}

func (d DryrunPRModifier) BranchPostFix() string {
	return "-dryrun"
}

func (d DryrunPRModifier) TitleTag() string {
	return "[DO-NOT-MERGE] "
}

func (d DryrunPRModifier) PostCommentPrefix() string {
	return "🧪 Auto-generated a DRYRUN PR: "
}

func NewDryrunPRModifier() DryrunPRModifier {
	return DryrunPRModifier{}
}
