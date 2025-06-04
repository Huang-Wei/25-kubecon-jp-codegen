package generator

import "context"

type MetadataService interface {
	GetClusters(ctx context.Context /*, selector */) ([]string, error)
}
