package rabata

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceRabataS3Bucket() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceRabataS3BucketRead,

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:     schema.TypeString,
				Required: true,
			},
			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"bucket_domain_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"bucket_regional_domain_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"region": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceRabataS3BucketRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	awsClient := meta.(*AWSClient) //nolint:forcetypeassert
	conn := awsClient.s3conn

	bucket := d.Get("bucket").(string) //nolint:forcetypeassert

	input := &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}

	log.Printf("[DEBUG] Reading S3 bucket: %s", input)

	_, err := conn.HeadBucketWithContext(ctx, input)
	if err != nil {
		return diag.Errorf("failed getting S3 bucket: %s Bucket: %q", err, bucket)
	}

	d.SetId(bucket)
	a := arn.ARN{
		Partition: "aws",
		Service:   "s3",
		Resource:  bucket,
	}.String()
	d.Set("arn", a) //nolint:errcheck

	bucketDomainName := awsClient.PartitionHostname(bucket + ".s3")

	d.Set("bucket_domain_name", bucketDomainName) //nolint:errcheck

	err = bucketLocation(ctx, awsClient, d, bucket)
	if err != nil {
		return diag.Errorf("error getting S3 Bucket location: %s", err)
	}

	d.Set("bucket_regional_domain_name", bucketDomainName) //nolint:errcheck

	return nil
}

func bucketLocation(ctx context.Context, client *AWSClient, d *schema.ResourceData, bucket string) error {
	region, err := s3manager.GetBucketRegionWithClient(
		ctx,
		client.s3conn,
		bucket,
		func(r *request.Request) {
			// By default, GetBucketRegion forces virtual host addressing, which
			// is not compatible with many non-AWS implementations. Instead, pass
			// the provider s3_force_path_style configuration, which defaults to
			// false, but allows override.
			r.Config.S3ForcePathStyle = client.s3conn.Config.S3ForcePathStyle
		},
	)
	if err != nil {
		return err
	}

	if err := d.Set("region", region); err != nil {
		return err
	}

	return nil
}
