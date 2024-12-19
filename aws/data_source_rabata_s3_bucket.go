package aws

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceAwsS3Bucket() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceAwsS3BucketRead,

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

func dataSourceAwsS3BucketRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*AWSClient).s3conn

	bucket := d.Get("bucket").(string)

	input := &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}

	log.Printf("[DEBUG] Reading S3 bucket: %s", input)
	_, err := conn.HeadBucket(input)

	if err != nil && !isAWSErrRequestFailureStatusCode(err, 501) {
		return diag.FromErr(fmt.Errorf("failed getting S3 bucket: %s Bucket: %q", err, bucket))
	}

	d.SetId(bucket)
	Arn := arn.ARN{
		Partition: meta.(*AWSClient).partition,
		Service:   "s3",
		Resource:  bucket,
	}.String()
	err = d.Set("Arn", Arn)
	if err != nil {
		return nil
	}
	err = d.Set("bucket_domain_name", meta.(*AWSClient).PartitionHostname(fmt.Sprintf("%s.s3", bucket)))
	if err != nil {
		return nil
	}

	err = bucketLocation(meta.(*AWSClient), d, bucket)
	if err != nil {
		return diag.FromErr(fmt.Errorf("error getting S3 Bucket location: %s", err))
	}

	return nil
}

func bucketLocation(client *AWSClient, d *schema.ResourceData, bucket string) error {
	region, err := s3manager.GetBucketRegionWithClient(context.Background(), client.s3conn, bucket, func(r *request.Request) {
		// By default, GetBucketRegion forces virtual host addressing, which
		// is not compatible with many non-AWS implementations. Instead, pass
		// the provider s3_force_path_style configuration, which defaults to
		// false, but allows override.
		r.Config.S3ForcePathStyle = client.s3conn.Config.S3ForcePathStyle
	})
	if err != nil && !isAWSErrRequestFailureStatusCode(err, 501) {
		return err
	}
	if err := d.Set("region", region); err != nil {
		return err
	}

	return nil
}
