package aws

import (
	"RabataTerraformProvider/aws/internal/rabata_endpoints"
	"fmt"
	"log"

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
	Token         string
	Region        string
	MaxRetries    int

	Endpoints map[string]string

	SkipCredsValidation     bool
	SkipRequestingAccountId bool
	SkipMetadataApiCheck    bool
	S3ForcePathStyle        bool
}

type AWSClient struct {
	accountid                 string
	dnsSuffix                 string
	partition                 string
	region                    string
	s3conn                    *s3.S3
	s3connUriCleaningDisabled *s3.S3
	supportedplatforms        []string
}

// PartitionHostname returns a hostname with the provider domain suffix for the partition
// e.g. PREFIX.amazonaws.com
// The prefix should not contain a trailing period.
func (client *AWSClient) PartitionHostname(prefix string) string {
	return fmt.Sprintf("%s.%s", prefix, client.dnsSuffix)
}

// RegionalHostname returns a hostname with the provider domain suffix for the region and partition
// e.g. PREFIX.us-west-2.amazonaws.com
// The prefix should not contain a trailing period.
func (client *AWSClient) RegionalHostname(prefix string) string {
	return fmt.Sprintf("%s.%s.%s", prefix, client.region, client.dnsSuffix)
}

// Client configures and returns a fully initialized AWSClient
func (c *Config) Client() (interface{}, error) {
	awsbaseConfig := &awsbase.Config{
		AccessKey:               c.AccessKey,
		CallerDocumentationURL:  "https://registry.terraform.io/providers/hashicorp/aws",
		CallerName:              "Terraform AWS Provider",
		CredsFilename:           c.CredsFilename,
		DebugLogging:            logging.IsDebugOrHigher(),
		MaxRetries:              c.MaxRetries,
		Profile:                 c.Profile,
		Region:                  c.Region,
		SecretKey:               c.SecretKey,
		SkipCredsValidation:     c.SkipCredsValidation,
		SkipMetadataApiCheck:    c.SkipMetadataApiCheck,
		SkipRequestingAccountId: c.SkipRequestingAccountId,
		StsEndpoint:             c.Endpoints["sts"],
		Token:                   c.Token,
		UserAgentProducts: []*awsbase.UserAgentProduct{
			{Name: "APN", Version: "1.0"},
			{Name: "HashiCorp", Version: "1.0"},
		},
	}

	sess, accountID, partition, err := awsbase.GetSessionWithAccountIDAndPartition(awsbaseConfig)
	if err != nil {
		return nil, fmt.Errorf("error configuring Terraform AWS Provider: %w", err)
	}

	if accountID == "" {
		log.Printf("[WARN] AWS account ID not found for provider. See https://www.terraform.io/docs/providers/aws/index.html#skip_requesting_account_id for implications.")
	}

	dnsSuffix, err := rabata_endpoints.RabataEndpoint(c.Region)
	if err != nil {
		return nil, fmt.Errorf("error configuring Terraform AWS Provider: %w", err)
	}

	client := &AWSClient{
		accountid: accountID,
		region:    c.Region,
		dnsSuffix: dnsSuffix,
		partition: partition,
	}

	// Services that require multiple client configurations
	s3Config := &aws.Config{
		Endpoint:         aws.String(c.Endpoints["s3"]),
		S3ForcePathStyle: aws.Bool(c.S3ForcePathStyle),
	}

	client.s3conn = s3.New(sess.Copy(s3Config))

	s3Config.DisableRestProtocolURICleaning = aws.Bool(true)
	client.s3connUriCleaningDisabled = s3.New(sess.Copy(s3Config))

	return client, nil
}
