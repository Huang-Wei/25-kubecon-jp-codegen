package generator

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/afero"

	"github.com/Huang-Wei/25-kubecon-jp-codegen/pkg/internal"
	"github.com/Huang-Wei/25-kubecon-jp/go/generated/infra/account"
	"github.com/Huang-Wei/25-kubecon-jp/go/generated/tenant/resource"
	"github.com/Huang-Wei/25-kubecon-jp/go/generated/tenant/selector"
	"github.com/Huang-Wei/25-kubecon-jp/go/generated/tenant/selector/operator"
)

type Codegen struct {
	fs afero.Fs
}

func NewCodegen() *Codegen {
	return &Codegen{
		fs: afero.NewOsFs(),
	}
}

// FanOutArtifacts render the eventual artifacts based on pre-processed Tenant and Infra tuples.
func (cg *Codegen) FanOutArtifacts(_ context.Context, dstDir string, accounts []*account.Account, tenantTuples []*internal.TenantTuple) error {
	tenantsDir := path.Join(dstDir, "_output", "tenants")
	// Delete the files that were auto-generated.
	_ = deleteGeneratedFiles(cg.fs, tenantsDir)

	for _, tuple := range tenantTuples {
		if tuple.ResourceConfig == nil {
			continue
		}

		// Deal with Buckets.
		if err := cg.iterateBuckets(tenantsDir, accounts, tuple); err != nil {
			return err
		}
		// TODO: deal with other fields.
	}
	return nil
}

func (cg *Codegen) iterateBuckets(tenantsDir string, accounts []*account.Account, tuple *internal.TenantTuple) error {
	if tuple.ResourceConfig == nil {
		return nil
	}

	for _, bucket := range tuple.ResourceConfig.Buckets {
		for _, act := range accounts {
			// Bypass non-matching account.
			if !envMatches(act.Tags, tuple) {
				continue
			}
			if !selectorMatches(act.Tags, bucket.Selector) {
				continue
			}

			// Start rendering the bucket towards the matched account.
			regionPath := path.Join(tenantsDir, "{{.CloudProvider}}-{{.AccountID}}/{{.RegionName}}")
			if err := cg.generateBucket(bucket, act, regionPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// PathContext represents the context for generating directory paths
type PathContext struct {
	CloudProvider string
	AccountID     string
	RegionName    string
}

// generateBucket generates a bucket config for a specific account and region
func (cg *Codegen) generateBucket(
	bucket *resource.Bucket,
	account *account.Account,
	templatePath string,
) error {
	// Generate the directory path using templating
	pathCtx := PathContext{
		CloudProvider: account.CloudProvider,
		AccountID:     account.AccountID,
		RegionName:    bucket.Region,
	}

	outputPath, err := cg.generateOutputPath(pathCtx, templatePath)
	if err != nil {
		return fmt.Errorf("failed to generate output path: %w", err)
	}

	if err := cg.fs.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", outputPath, err)
	}

	// Render bucket.yaml.tpl
	if out, err := renderBucket(bucket, account); err != nil {
		return fmt.Errorf("failed to render buckets template: %w", err)
	} else {
		outputPath = filepath.Join(outputPath, fmt.Sprintf("%s-bucket.yaml", bucket.Name))
		if err := afero.WriteFile(cg.fs, outputPath, []byte(out), 0755); err != nil {
			return fmt.Errorf("failed to write file %s: %w", outputPath, err)
		}
	}

	return nil
}

// generateOutputPath generates the output directory path using Go templating
func (cg *Codegen) generateOutputPath(pathCtx PathContext, templatesPath string) (string, error) {
	tmpl, err := template.New("path").Parse(templatesPath)
	if err != nil {
		return "", fmt.Errorf("failed to parse path template: %w", err)
	}

	var pathBuilder strings.Builder
	if err := tmpl.Execute(&pathBuilder, pathCtx); err != nil {
		return "", fmt.Errorf("failed to execute path template: %w", err)
	}

	return pathBuilder.String(), nil
}

func envMatches(accountTags map[string]string, tuple *internal.TenantTuple) bool {
	env := accountTags["env"]
	return env == "" || env == tuple.Env
}

func selectorMatches(accountTags map[string]string, selector []*selector.Requirment) bool {
	if len(selector) == 0 {
		return true
	}

	// All requirements must be satisfied for the selector to match
	for _, req := range selector {
		if !requirementMatches(accountTags, req) {
			return false
		}
	}

	return true
}

func requirementMatches(tags map[string]string, req *selector.Requirment) bool {
	keyStr := string(req.Key) // Convert key.Key to string
	tagValue, exists := tags[keyStr]

	switch req.Operator {
	case operator.In:
		// Key must exist and its value must be in the values array
		if !exists {
			return false
		}
		return contains(req.Values, tagValue)

	case operator.NotIn:
		// If key doesn't exist, requirement is satisfied
		// If key exists, its value must not be in the values array
		if !exists {
			return true
		}
		return !contains(req.Values, tagValue)

	case operator.Exists:
		// Key must exist (regardless of value)
		return exists

	case operator.DoesNotExist:
		// Key must not exist
		return !exists

	default:
		// Unknown operator, fail safe
		return false
	}
}

// helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func deleteGeneratedFiles(fs afero.Fs, dstDir string) error {
	return afero.Walk(fs, dstDir, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		file, err := fs.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Read the first line of the file
		scanner := bufio.NewScanner(file)
		if scanner.Scan() {
			firstLine := scanner.Text()
			// Check if the first line starts with "# Code generated"
			if strings.HasPrefix(firstLine, "# Code generated") {
				// Delete the file
				if err := fs.Remove(path); err != nil {
					return err
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}

		return nil
	})
}
