package git

import (
	"context"
	"os/exec"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"

	"github.com/Huang-Wei/25-kubecon-jp-codegen/pkg/internal"
	"github.com/Huang-Wei/25-kubecon-jp/go/generated/tenant/resource"
	"github.com/Huang-Wei/25-kubecon-jp/go/generated/tenant/selector"
)

// pkl-gen-go evaluates an empty Listing as a non-nil slice instead of nil.
var emptySelector = []*selector.Requirment{}

// checkBinaryExists checks if a binary exists in the system PATH
func checkBinaryExists(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

// requireBinaries checks for required binaries and skips the test if any are missing
func requireBinaries(t *testing.T, binaries ...string) {
	t.Helper()

	var missing []string
	for _, binary := range binaries {
		if !checkBinaryExists(binary) {
			missing = append(missing, binary)
		}
	}

	if len(missing) > 0 {
		if len(missing) == 1 {
			t.Skipf("Required binary %q not found in PATH", missing[0])
		} else {
			t.Skipf("Required binaries not found in PATH: %v", missing)
		}
	}
}

func Test_parseTenants(t *testing.T) {
	// LoadFromPath() requires 'pkl' and 'pkl-gen-go' to be present on PATH.
	requireBinaries(t, "pkl", "pkl-gen-go")

	ctx := context.Background()

	tests := []struct {
		name     string
		fs       afero.Fs
		rootPath string
		want     []*internal.TenantTuple
		wantErr  bool
	}{
		{
			name:     "empty input",
			fs:       afero.NewMemMapFs(),
			rootPath: "/",
		},
		{
			name:     "read tenant files in testdata/",
			fs:       afero.NewOsFs(),
			rootPath: "testdata/tenants",
			want: []*internal.TenantTuple{
				{
					TenantID: "bar",
					Env:      "prod",
					ResourceConfig: &resource.ResourceConfig{
						Kubernetes: &resource.Kubernetes{
							Namespaces: []string{"bar"},
							Selector:   emptySelector,
						},
						Buckets: []*resource.Bucket{
							{Name: "bar-bucket-", Selector: emptySelector},
						},
					},
				},
				{
					TenantID: "foo",
					Env:      "dev",
					ResourceConfig: &resource.ResourceConfig{
						Kubernetes: &resource.Kubernetes{
							Namespaces: []string{"foo1", "foo2"},
							Selector:   emptySelector,
						},
						Buckets: []*resource.Bucket{
							{Name: "foo-bucket-", Selector: emptySelector},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTenants(ctx, tt.fs, tt.rootPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTenants() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("parseTenants() = (-got +want)\n%s", diff)
			}
		})
	}
}
