package internal

import (
	"github.com/Huang-Wei/25-kubecon-jp/go/generated/tenant/resource"
)

type TenantTuple struct {
	TenantID       string
	Env            string
	ResourceConfig *resource.ResourceConfig
}
