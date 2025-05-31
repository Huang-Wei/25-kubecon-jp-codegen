package git

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/prow/pkg/git/v2"
	"sigs.k8s.io/prow/pkg/github"
)

var _ Worker = &ResourceWorker{}

func NewResourceWorker(opts ...ResourceWorkerOption) *ResourceWorker {
	resWorker := &ResourceWorker{}
	for _, opt := range opts {
		opt(resWorker)
	}
	return resWorker
}

type ResourceWorker struct {
	gc      git.ClientFactory
	ghc     github.Client
	botUser *github.UserData
	email   string
	ghToken string
	prune   bool
	logger  logr.Logger
}

func (r *ResourceWorker) AddLabel(org, repo string, number int, label string) error {
	return r.ghc.AddLabel(org, repo, number, label)
}

func (r *ResourceWorker) CreatePullRequestAllTenants(_ context.Context, _ CodegenFunc) error {
	// TODO implement me
	panic("implement me")
}

func (r *ResourceWorker) Logger() logr.Logger {
	return r.logger
}

type ResourceWorkerOption func(*ResourceWorker)

func WithGC(gc git.ClientFactory) ResourceWorkerOption {
	return func(rw *ResourceWorker) {
		rw.gc = gc
	}
}

func WithGHC(ghc github.Client) ResourceWorkerOption {
	return func(rw *ResourceWorker) {
		rw.ghc = ghc
	}
}

func WithBotUser(botUser *github.UserData) ResourceWorkerOption {
	return func(rw *ResourceWorker) {
		rw.botUser = botUser
	}
}

func WithEmail(email string) ResourceWorkerOption {
	return func(rw *ResourceWorker) {
		rw.email = email
	}
}

func WithGHToken(token string) ResourceWorkerOption {
	return func(rw *ResourceWorker) {
		rw.ghToken = token
	}
}

func WithPrune(prune bool) ResourceWorkerOption {
	return func(rw *ResourceWorker) {
		rw.prune = prune
	}
}

func WithLogger(logger logr.Logger) ResourceWorkerOption {
	return func(rw *ResourceWorker) {
		rw.logger = logger
	}
}
