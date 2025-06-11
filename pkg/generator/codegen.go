package generator

import (
	"bufio"
	"context"
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

const (
	TenantsOutputDir = "_output/tenants"
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
	tenantsDir := path.Join(dstDir, TenantsOutputDir)
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

	// Generate kustomization.yaml to include all auto-generated files.
	if err := generateKustomizationFiles(cg.fs, tenantsDir); err != nil {
		return err
	}

	return nil
}

func (cg *Codegen) iterateBuckets(tenantsDir string, accounts []*account.Account, tuple *internal.TenantTuple) error {
	if tuple.ResourceConfig == nil {
		return nil
	}

	for _, bucket := range tuple.ResourceConfig.Buckets {
		for _, act := range accounts {
			// Env is an implicit matching criteria.
			if !envMatches(act.Tags, tuple) {
				continue
			}
			if !selectorMatches(act.Tags, bucket.Selector) {
				continue
			}

			// Start rendering the bucket towards the matched account.
			regionPath := path.Join(tenantsDir, tuple.TenantID, "{{.CloudProvider}}-{{.AccountID}}/{{.RegionName}}")
			if err := cg.generateBucket(bucket, act, regionPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func generateKustomizationFiles(fs afero.Fs, dir string) error {
	return afero.Walk(fs, dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		return generateOneKustomizationFile(fs, path)
	})
}

func generateOneKustomizationFile(fs afero.Fs, dir string) error {
	entries, err := afero.ReadDir(fs, dir)
	if err != nil {
		return err
	}

	var yamlFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		// Include .yaml files, but exclude kustomization.yaml.
		if strings.HasSuffix(filename, ".yaml") && filename != "kustomization.yaml" {
			yamlFiles = append(yamlFiles, filename)
		}
	}

	if len(yamlFiles) == 0 {
		return nil
	}

	// Strip prefix string ends with TenantsOutputDir
	idx := strings.Index(dir, TenantsOutputDir)
	if idx == -1 {
		return fmt.Errorf("[internal error] '%s' doesn't look like a tenant directory", dir)
	}
	namePrefix := dir[idx+len(TenantsOutputDir):]
	// Remove extra leading '/':
	for len(namePrefix) > 0 && namePrefix[0] == '/' {
		namePrefix = namePrefix[1:]
	}
	namePrefix = strings.Join(strings.Split(namePrefix, "/"), "-")
	out, err := renderKustomization(namePrefix, yamlFiles)
	if err != nil {
		return err
	}
	// Write <provider>-bucket.yaml
	outputPath := filepath.Join(dir, "kustomization.yaml")
	if err := afero.WriteFile(fs, outputPath, []byte(out), 0755); err != nil {
		return fmt.Errorf("failed to write file %s: %w", outputPath, err)
	}

	return nil
}

// pathContext represents the context for generating directory paths
type pathContext struct {
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
	pathCtx := pathContext{
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

	// Render <provider>-bucket.yaml.tpl
	out, err := renderBucket(bucket, account)
	if err != nil {
		return fmt.Errorf("failed to render buckets template: %w", err)
	}
	// Write bucket yaml
	outputPath = filepath.Join(outputPath, fmt.Sprintf("bucket-%s.yaml", bucket.Name))
	if err := afero.WriteFile(cg.fs, outputPath, []byte(out), 0755); err != nil {
		return fmt.Errorf("failed to write file %s: %w", outputPath, err)
	}

	return nil
}

// generateOutputPath generates the output directory path using Go templating
func (cg *Codegen) generateOutputPath(pathCtx pathContext, templatesPath string) (string, error) {
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
		// If key doesn't exist, return true
		// Otherwise, its value must be in the values array
		return !exists || contains(req.Values, tagValue)

	case operator.NotIn:
		// If key doesn't exist, return true
		// Otherwise, its value must not be in the values array
		return !exists || !contains(req.Values, tagValue)

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

// delete all previously-generated files that start with
// # Code generated
func deleteGeneratedFiles(fs afero.Fs, dstDir string) error {
	return afero.Walk(fs, dstDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
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
