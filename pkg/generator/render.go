package generator

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/Huang-Wei/25-kubecon-jp/go/generated/infra/account"
	"github.com/Huang-Wei/25-kubecon-jp/go/generated/tenant/resource"
)

//go:embed templates/tenants/non-k8s/aws-bucket.yaml.tpl
var embedAWSBucket string
var awsBucketTpl = template.Must(template.New("aws-bucket").Parse(embedAWSBucket))

//go:embed templates/tenants/non-k8s/gcp-bucket.yaml.tpl
var embedGCPBucket string
var gcpBucketTpl = template.Must(template.New("gcp-bucket").Funcs(customFuncMap()).Parse(embedGCPBucket))

func customFuncMap() template.FuncMap {
	return template.FuncMap{
		"toGCPRegion": toGCPRegion,
	}
}

func renderBucket(bucket *resource.Bucket, account *account.Account) (string, error) {
	var tpl *template.Template
	cloudProvider := account.CloudProvider
	switch cloudProvider {
	case "aws":
		tpl = awsBucketTpl
	case "gcp":
		tpl = gcpBucketTpl
	default:
		return "", fmt.Errorf("unsupported cloud provider: %s", cloudProvider)
	}

	buf := bytes.NewBuffer(nil)
	err := tpl.Execute(buf, bucket)
	if err != nil {
		return "", fmt.Errorf("rendering error: %w", err)
	}

	return buf.String(), nil
}

// ToGCPRegion converts AWS-style region name to GCP format
// AWS format: "us-east-1", "eu-west-2", "ap-southeast-1"
// GCP format: "us-east1", "europe-west2", "asia-southeast1"
func toGCPRegion(awsRegion string) string {
	// Handle special region mappings first
	regionMap := map[string]string{
		// US regions
		"us-east-1":     "us-east1",
		"us-east-2":     "us-east4",
		"us-west-1":     "us-west2",
		"us-west-2":     "us-west1",
		"us-gov-east-1": "us-east1", // Government regions map to regular regions
		"us-gov-west-1": "us-west1",

		// Europe regions
		"eu-west-1":    "europe-west2",
		"eu-west-2":    "europe-west2",
		"eu-west-3":    "europe-west9",
		"eu-central-1": "europe-west3",
		"eu-north-1":   "europe-north1",
		"eu-south-1":   "europe-southwest1",

		// Asia Pacific regions
		"ap-southeast-1": "asia-southeast1",
		"ap-southeast-2": "asia-southeast2",
		"ap-northeast-1": "asia-northeast1",
		"ap-northeast-2": "asia-northeast3",
		"ap-northeast-3": "asia-northeast2",
		"ap-south-1":     "asia-south1",
		"ap-east-1":      "asia-east2",

		// Other regions
		"ca-central-1": "northamerica-northeast1",
		"sa-east-1":    "southamerica-east1",
		"af-south-1":   "africa-south1",
		"me-south-1":   "me-west1",
	}

	// Check if we have a direct mapping
	if gcpRegion, exists := regionMap[awsRegion]; exists {
		return gcpRegion
	}

	// Generic conversion for regions not in the map
	// This handles the basic pattern transformation
	return convertGenericRegion(awsRegion)
}

// convertGenericRegion handles basic pattern conversion
func convertGenericRegion(awsRegion string) string {
	// Replace common prefixes
	region := awsRegion

	// Convert eu- to europe-
	if strings.HasPrefix(region, "eu-") {
		region = strings.Replace(region, "eu-", "europe-", 1)
	}

	// Convert ap- to asia-
	if strings.HasPrefix(region, "ap-") {
		region = strings.Replace(region, "ap-", "asia-", 1)
	}

	// Convert ca- to northamerica-
	if strings.HasPrefix(region, "ca-") {
		region = strings.Replace(region, "ca-", "northamerica-", 1)
	}

	// Convert sa- to southamerica-
	if strings.HasPrefix(region, "sa-") {
		region = strings.Replace(region, "sa-", "southamerica-", 1)
	}

	// Remove the last hyphen and number, then add number without hyphen
	// e.g., "us-east-1" -> "us-east1"
	parts := strings.Split(region, "-")
	if len(parts) >= 3 {
		// Rejoin all parts except the last one, then append the last part
		base := strings.Join(parts[:len(parts)-1], "-")
		number := parts[len(parts)-1]
		region = base + number
	}

	return region
}
