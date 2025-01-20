package rabata

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	awsbase "github.com/hashicorp/aws-sdk-go-base"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
)

type Config struct {
	AccessKey     string
	SecretKey     string
	CredsFilename string
	Profile       string
	Region        string
	MaxRetries    int

	Endpoints map[string]string
	Insecure  bool

	S3ForcePathStyle bool

	terraformVersion string
}

type AWSClient struct {
	dnsSuffix                 string
	region                    string
	s3conn                    *s3.S3
	s3connURICleaningDisabled *s3.S3
}

// PartitionHostname returns a hostname with the provider domain suffix for the partition
// e.g. PREFIX.rabata.io
// The prefix should not contain a trailing period.
func (client *AWSClient) PartitionHostname(prefix string) string {
	return fmt.Sprintf("%s.%s", prefix, client.dnsSuffix)
}

// Client configures and returns a fully initialized AWSClient.
func (c *Config) Client() (*AWSClient, error) {
	awsbaseConfig := &awsbase.Config{
		AccessKey:               c.AccessKey,
		CallerDocumentationURL:  "https://registry.terraform.io/providers/rabataio/rabata",
		CallerName:              "Terraform Rabata Provider",
		CredsFilename:           c.CredsFilename,
		DebugLogging:            logging.IsDebugOrHigher(),
		Insecure:                c.Insecure,
		MaxRetries:              c.MaxRetries,
		Profile:                 c.Profile,
		Region:                  c.Region,
		SecretKey:               c.SecretKey,
		SkipCredsValidation:     true,
		SkipMetadataApiCheck:    true,
		SkipRequestingAccountId: true,
		UserAgentProducts: []*awsbase.UserAgentProduct{
			{Name: "APN", Version: "1.0"},
			{Name: "HashiCorp", Version: "1.0"},
			{
				Name:    "Terraform",
				Version: c.terraformVersion,
				Extra:   []string{"+https://www.terraform.io"},
			},
		},
	}

	sess, err := awsbase.GetSession(awsbaseConfig)
	if err != nil {
		return nil, fmt.Errorf("error configuring Terraform AWS Provider: %w", err)
	}

	dnsSuffix := getDNSSuffix(c.Region)

	client := &AWSClient{
		region:    c.Region,
		dnsSuffix: dnsSuffix,
	}

	// Services that require multiple client configurations
	s3Config := &aws.Config{
		Endpoint:                aws.String(c.Endpoints["s3"]),
		S3ForcePathStyle:        aws.Bool(c.S3ForcePathStyle),
		DisableComputeChecksums: aws.Bool(true),
	}

	client.s3conn = s3.New(sess.Copy(s3Config))

	s3Config.DisableRestProtocolURICleaning = aws.Bool(true)
	client.s3connURICleaningDisabled = s3.New(sess.Copy(s3Config))

	return client, nil
}
