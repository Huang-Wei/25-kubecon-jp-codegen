package generator

import (
	"context"

	"github.com/spf13/afero"
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
func (cg *Codegen) FanOutArtifacts(_ context.Context, _ string) error {
	return nil
}
