package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/afero"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/prow/pkg/git/v2"
	"sigs.k8s.io/prow/pkg/github"

	"github.com/Huang-Wei/25-kubecon-jp-codegen/pkg/internal"
	"github.com/Huang-Wei/25-kubecon-jp/go/generated/infra/account"
	"github.com/Huang-Wei/25-kubecon-jp/go/generated/tenant/resource"
)

var (
	checkoutBranchFmt = "auto-checkout-%d-to-%s%s"
	GitHubURL         = "https://github.com"

	ErrNothingToCommit          = errors.New("nothing to commit")
	ErrPullRequestAlreadyExists = errors.New("pull request already exists")
)

var _ Worker = &ResourceWorker{}

type ResourceWorker struct {
	gc      git.ClientFactory
	ghc     github.Client
	botUser *github.UserData
	email   string
	logger  logr.Logger
}

func (r *ResourceWorker) GetPullRequest(org, repo string, number int) (*github.PullRequest, error) {
	return r.ghc.GetPullRequest(org, repo, number)
}

func (r *ResourceWorker) CreatePullRequest(
	ctx context.Context,
	upstreamRepo *GHRepo,
	prModifier PullRequestModifier,
	repo string,
	accounts []*account.Account,
	tenantTuples []*internal.TenantTuple,
	codegenFunc CodegenFunc,
) error {
	downstreamRepo, err := r.CreateDownstreamRepo(repo, upstreamRepo)
	if err != nil {
		return err
	}
	defer func() {
		if err := downstreamRepo.Client.Clean(); err != nil {
			r.logger.Error(err, "error cleaning up repo.")
		}
	}()

	downstreamBranch, err := r.CreateDownstreamBranch(upstreamRepo, downstreamRepo, prModifier)
	if err != nil {
		if errors.Is(err, ErrPullRequestAlreadyExists) {
			return nil
		}
		return err
	}

	dstDir := downstreamRepo.Client.Directory()

	// PR generation logic starts.
	startPRGen := time.Now()
	if err := codegenFunc(ctx, dstDir, accounts, tenantTuples); err != nil {
		return fmt.Errorf("failed to generate PR: %w", err)
	}
	r.logger.WithValues("duration", time.Since(startPRGen)).Info("PR generation completed.")
	// PR generation logic ends.

	prNum, err := r.CommitChanges(dstDir, downstreamBranch, upstreamRepo, downstreamRepo, prModifier)
	if err != nil {
		return err
	}
	// Close the pr if needed.
	if prModifier.TearDown() {
		_ = r.ghc.ClosePullRequest(downstreamRepo.Org, downstreamRepo.Name, prNum)
	}
	return nil
}

func (r *ResourceWorker) AddLabel(org, repo string, number int, label string) error {
	return r.ghc.AddLabel(org, repo, number, label)
}

func (r *ResourceWorker) Logger() logr.Logger {
	return r.logger
}

// FetchUpstreamConfigs fetches and parses the user input configured in upstream repo.
func (r *ResourceWorker) FetchUpstreamConfigs(ctx context.Context, upstreamRepo *GHRepo) ([]*account.Account, []*internal.TenantTuple, error) {
	_, err := r.ghc.GetPullRequestChanges(upstreamRepo.Org, upstreamRepo.Name, upstreamRepo.PullRequestNumber)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot list PR changes: %w", err)
	}

	uRepoClient, err := r.gc.ClientFor(upstreamRepo.Org, upstreamRepo.Name)
	if err != nil {
		return nil, nil, err
	}
	if err := uRepoClient.Checkout(upstreamRepo.MergeSHA); err != nil {
		return nil, nil, err
	}
	if err := uRepoClient.CheckoutNewBranch(fmt.Sprintf("src-%v", upstreamRepo.PullRequestNumber)); err != nil {
		return nil, nil, err
	}
	uDir := uRepoClient.Directory()

	// Parse infra/account.pkl
	accounts, err := parseAccounts(ctx, afero.NewOsFs(), filepath.Join(uDir, "infra"))
	if err != nil {
		return nil, nil, err
	}

	// Iterate upstream repo's `tenants/` folder.
	tenantTuples, err := parseTenants(ctx, afero.NewOsFs(), filepath.Join(uDir, "tenants"))
	if err != nil {
		return nil, nil, err
	}

	return accounts, tenantTuples, nil
}

// parseAccounts parses `infra/account.pkl` and return a list of Account.
func parseAccounts(ctx context.Context, fs afero.Fs, rootPath string) ([]*account.Account, error) {
	accountPklPath := filepath.Join(rootPath, "account.pkl")
	// Check if file exists
	exists, err := afero.Exists(fs, accountPklPath)
	if err != nil {
		return nil, fmt.Errorf("failed to check if file exists %s: %w", accountPklPath, err)
	}
	if !exists {
		return nil, fmt.Errorf("account.pkl not found at %s", accountPklPath)
	}

	accountConfig, err := account.LoadFromPath(ctx, accountPklPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load account from %s: %w", accountPklPath, err)
	}

	return accountConfig.Accounts, nil
}

// parseTenants parses the `tenants/` folder to read resource.pkl and convert into tenant tuples.
func parseTenants(ctx context.Context, fs afero.Fs, rootPath string) ([]*internal.TenantTuple, error) {
	var tenantTuples []*internal.TenantTuple

	// Walk through the root directory
	err := afero.Walk(fs, rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip if it's not a file or not named 'resource.pkl'
		if info.IsDir() || info.Name() != "resource.pkl" {
			return nil
		}

		// Get relative path from root
		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Extract tenant_id and env from the path
		// Expected structure: tenants/<tenant_id>/<env>/resource.pkl
		pathParts := strings.Split(relPath, "/")
		if len(pathParts) < 3 {
			// Skip if path doesn't match expected structure
			return nil
		}

		// Parse resource.pkl
		resourceConfig, err := resource.LoadFromPath(ctx, path)
		if err != nil {
			return fmt.Errorf("failed to parse '%s': %w", path, err)
		}

		// Add to tenants slice
		tenantTuples = append(tenantTuples, &internal.TenantTuple{
			TenantID:       pathParts[0],
			Env:            pathParts[1],
			ResourceConfig: resourceConfig,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory tree: %w", err)
	}

	return tenantTuples, nil
}

func (r *ResourceWorker) CreateDownstreamRepo(repo string, upstreamRepo *GHRepo) (*GHRepo, error) {
	startTime := time.Now()
	dOrg, dRepo := strings.Split(repo, "/")[0], strings.Split(repo, "/")[1]
	// Ensure downstream repo's fork exists.
	forkName, err := r.ghc.EnsureFork(r.botUser.Login, dOrg, dRepo)
	if err != nil {
		r.logger.Error(err, "failed to ensure fork exists")
		resp := fmt.Sprintf("cannot fork %s/%s: %v", upstreamRepo.Org, upstreamRepo.Name, err)
		if err = r.ghc.CreateComment(upstreamRepo.Org, upstreamRepo.Name, upstreamRepo.PullRequestNumber, resp); err != nil {
			return nil, err
		}
		return nil, errors.New(resp)
	}

	dRepoClient, err := r.gc.ClientFor(dOrg, dRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to get git client for %s/%s: %w", upstreamRepo.Org, forkName, err)
	}
	r.logger.WithValues("duration", time.Since(startTime)).Info("Finished forking target repo.")

	return &GHRepo{
		Org:    dOrg,
		Name:   dRepo,
		Client: dRepoClient,
	}, nil
}

func (r *ResourceWorker) CreateDownstreamBranch(upstreamRepo, downstreamRepo *GHRepo, prModifier PullRequestModifier) (*DownstreamBranch, error) {
	targetBranch, newBranch := r.GetBranchNames(upstreamRepo, downstreamRepo, prModifier)

	startTime := time.Now()
	if err := downstreamRepo.Client.Checkout(targetBranch); err != nil {
		r.logger.Error(err, "failed to checkout target branch")
		resp := fmt.Sprintf("cannot checkout `%s`: %v", targetBranch, err)
		return nil, r.ghc.CreateComment(upstreamRepo.Org, upstreamRepo.Name, upstreamRepo.PullRequestNumber, resp)
	}
	r.logger.WithValues("duration", time.Since(startTime)).Info("Checked out target branch.")

	// Set user.name and user.email.
	if err := downstreamRepo.Client.Config("user.name", r.botUser.Login); err != nil {
		return nil, fmt.Errorf("failed to configure git user: %w", err)
	}
	email := r.email
	if email == "" {
		email = r.botUser.Email
	}
	if err := downstreamRepo.Client.Config("user.email", email); err != nil {
		return nil, fmt.Errorf("failed to configure git email: %w", err)
	}

	// New branch for the automated PR.
	// Check if there is already a PR created. If so, find the PR and link to it.
	prs, err := r.ghc.GetPullRequests(downstreamRepo.Org, downstreamRepo.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull requests for %s/%s: %w", downstreamRepo.Org, downstreamRepo.Name, err)
	}
	for _, pr := range prs {
		r.logger.Info(fmt.Sprintf("pull request reference: %v", pr.Head.Ref))
		if pr.Head.Ref == newBranch {
			r.logger.WithValues("preexisting_pr", pr.HTMLURL).Info("PR already exists")
			resp := fmt.Sprintf("Looks like #%d has already been created in %s", pr.Number, pr.HTMLURL)
			if err = r.ghc.CreateComment(upstreamRepo.Org, upstreamRepo.Name, upstreamRepo.PullRequestNumber, resp); err != nil {
				return nil, err
			}
			return &DownstreamBranch{
				ExistingPullRequestNumber: pr.Number,
			}, ErrPullRequestAlreadyExists
		}
	}
	// Create the branch for the auto-generated PR.
	if err := downstreamRepo.Client.CheckoutNewBranch(newBranch); err != nil {
		return nil, fmt.Errorf("failed to checkout %s: %w", newBranch, err)
	}

	return &DownstreamBranch{
		TargetBranch: targetBranch,
		NewBranch:    newBranch,
	}, nil
}

func (r *ResourceWorker) GetBranchNames(upstreamRepo, _ *GHRepo, prModifier PullRequestModifier) (string, string) {
	// If the downstream repo's default branch is "master", tweak it here.
	targetBranch := "main"

	newBranch := fmt.Sprintf(checkoutBranchFmt, upstreamRepo.PullRequestNumber, targetBranch, prModifier.BranchPostFix())
	return targetBranch, newBranch
}

func (r *ResourceWorker) CommitChanges(dstDir string, downstreamBranch *DownstreamBranch, upstreamRepo, downstreamRepo *GHRepo, prModifier PullRequestModifier) (int, error) {
	commitMsg := "autogenerated"
	// There is a NPE issue when using r.Commit(). Hack it around..
	// if err := r.Commit("Fake changes on Mitosis", ""); err != nil {
	if err := commit(dstDir, commitMsg); err != nil {
		if errors.Is(err, ErrNothingToCommit) {
			return 0, r.ghc.CreateComment(upstreamRepo.Org, upstreamRepo.Name, upstreamRepo.PullRequestNumber, prModifier.NoopMsg(*downstreamRepo))
		}

		errs := []error{fmt.Errorf("failed to `git add & git commit`: %w", err)}
		r.logger.Error(err, "failed to apply PR on top of target branch")
		resp := fmt.Sprintf("#%d failed to apply on top of branch %q:\n```\n%v\n```", upstreamRepo.PullRequestNumber, downstreamBranch.TargetBranch, err)
		if err := r.ghc.CreateComment(upstreamRepo.Org, upstreamRepo.Name, upstreamRepo.PullRequestNumber, resp); err != nil {
			errs = append(errs, fmt.Errorf("failed to create comment: %w", err))
		}
		return 0, utilerrors.NewAggregate(errs)
	}
	// Push the new branch in the bot's fork.
	if err := downstreamRepo.Client.PushToFork(downstreamBranch.NewBranch, true); err != nil {
		r.logger.Error(err, "failed to push auto-generated changes to GitHub")
		resp := fmt.Sprintf("failed to push auto-generated changes in GitHub: %v", err)
		return 0, utilerrors.NewAggregate([]error{err, r.ghc.CreateComment(upstreamRepo.Org, upstreamRepo.Name, upstreamRepo.PullRequestNumber, resp)})
	}
	// Last step to create the PR.
	from := fmt.Sprintf("%s/%s/pull/%v", upstreamRepo.Org, upstreamRepo.Name, upstreamRepo.PullRequestNumber)
	title := fmt.Sprintf("%s🇯🇵 25 KubeCon CodeGen from %s", prModifier.TitleTag(), from)
	head := fmt.Sprintf("%s:%s", r.botUser.Login, downstreamBranch.NewBranch)
	createdNum, err := r.ghc.CreatePullRequest(downstreamRepo.Org, downstreamRepo.Name, title,
		fmt.Sprintf("This is an auto-generated PR via prow bot from %s.", from), head, downstreamBranch.TargetBranch, true)
	if err != nil {
		r.logger.Error(err, "failed to create new pull request")
		resp := fmt.Sprintf("new pull request could not be created: %v", err)
		return 0, utilerrors.NewAggregate([]error{err, r.ghc.CreateComment(upstreamRepo.Org, upstreamRepo.Name, upstreamRepo.PullRequestNumber, resp)})
	}

	// Comment on the original PR about the successful creation of auto-gen PR.
	resp := fmt.Sprintf(`%s%s/%s/%s/pull/%d.`, prModifier.PostCommentPrefix(), GitHubURL, downstreamRepo.Org, downstreamRepo.Name, createdNum)
	if err := r.ghc.CreateComment(upstreamRepo.Org, upstreamRepo.Name, upstreamRepo.PullRequestNumber, resp); err != nil {
		return 0, fmt.Errorf("failed to create comment: %w", err)
	}
	return createdNum, nil
}

type ResourceWorkerOption func(*ResourceWorker)

func NewResourceWorker(opts ...ResourceWorkerOption) *ResourceWorker {
	resWorker := &ResourceWorker{}
	for _, opt := range opts {
		opt(resWorker)
	}
	return resWorker
}

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

func WithLogger(logger logr.Logger) ResourceWorkerOption {
	return func(rw *ResourceWorker) {
		rw.logger = logger
	}
}
