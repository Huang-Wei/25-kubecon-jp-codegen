package git

import (
	"sigs.k8s.io/prow/pkg/git/v2"
)

type GHRepo struct {
	Org               string
	Name              string
	PullRequestNumber int
	MergeSHA          string
	Client            git.RepoClient
}

type DownstreamBranch struct {
	TargetBranch              string
	NewBranch                 string
	ExistingPullRequestNumber int
}
