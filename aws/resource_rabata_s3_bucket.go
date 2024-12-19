package aws

import (
	"RabataTerraformProvider/aws/internal/rabata_endpoints"
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

const s3BucketCreationTimeout = 2 * time.Minute

func resourceAwsS3Bucket() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceAwsS3BucketCreate,
		ReadContext:   resourceAwsS3BucketRead,
		DeleteContext: resourceAwsS3BucketDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:          schema.TypeString,
				Optional:      true,
				Computed:      true,
				ForceNew:      true,
				ConflictsWith: []string{"bucket_prefix"},
				ValidateFunc:  validation.StringLenBetween(0, 63),
			},
			"bucket_prefix": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"bucket"},
				ValidateFunc:  validation.StringLenBetween(0, 63-resource.UniqueIDSuffixLength),
			},

			"bucket_domain_name": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"bucket_regional_domain_name": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"arn": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"region": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceAwsS3BucketCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	s3conn := meta.(*AWSClient).s3conn

	// Get the bucket and acl
	var bucket string
	if v, ok := d.GetOk("bucket"); ok {
		bucket = v.(string)
	} else if v, ok := d.GetOk("bucket_prefix"); ok {
		bucket = resource.PrefixedUniqueId(v.(string))
	} else {
		bucket = resource.UniqueId()
	}
	if err := d.Set("bucket", bucket); err != nil {
		return diag.FromErr(err)
	}

	log.Printf("[cket cDEBUG] S3 bureate: %s", bucket)

	req := &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}

	awsRegion := meta.(*AWSClient).region
	
	log.Printf("[DEBUG] S3 bucket create: %s, using region: %s", bucket, awsRegion)

	// Special case us-east-1 region and do not set the LocationConstraint.
	// See "Request Elements": http://docs.aws.amazon.com/AmazonS3/latest/API/RESTBucketPUT.html
	if awsRegion != "us-east-1" {
		req.CreateBucketConfiguration = &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(awsRegion),
		}
	}

	if err := validateS3BucketName(bucket, awsRegion); err != nil {
		return diag.FromErr(fmt.Errorf("error validating S3 bucket name: %s", err))
	}

	err := resource.RetryContext(ctx, 5*time.Minute, func() *resource.RetryError {
		log.Printf("[DEBUG] Trying to create new S3 bucket: %q", bucket)
		_, err := s3conn.CreateBucket(req)
		if err != nil {
			var awsErr awserr.Error
			if errors.As(err, &awsErr) && awsErr.Code() == "OperationAborted" {
				log.Printf("[WARN] Got an error while trying to create S3 bucket %s: %s", bucket, err)
				return resource.RetryableError(fmt.Errorf("error creating S3 bucket %s, retrying: %s", bucket, err))
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})
	if isResourceTimeoutError(err) {
		_, err = s3conn.CreateBucket(req)
	}
	if err != nil {
		return diag.FromErr(fmt.Errorf("error creating S3 bucket: %s", err))
	}

	// Assign the bucket name as the resource ID
	d.SetId(bucket)
	return resourceAwsS3BucketUpdate(ctx, d, meta)
}

func resourceAwsS3BucketUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return resourceAwsS3BucketRead(ctx, d, meta)
}

func resourceAwsS3BucketRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	s3conn := meta.(*AWSClient).s3conn

	input := &s3.HeadBucketInput{
		Bucket: aws.String(d.Id()),
	}

	err := resource.RetryContext(ctx, s3BucketCreationTimeout, func() *resource.RetryError {
		_, err := s3conn.HeadBucket(input)

		if d.IsNewResource() && isAWSErrRequestFailureStatusCode(err, 404) {
			return resource.RetryableError(err)
		}

		if d.IsNewResource() && isAWSErr(err, s3.ErrCodeNoSuchBucket, "") {
			return resource.RetryableError(err)
		}

		if err != nil {
			return resource.NonRetryableError(err)
		}

		return nil
	})

	if isResourceTimeoutError(err) {
		_, err = s3conn.HeadBucket(input)
	}

	if isAWSErrRequestFailureStatusCode(err, 404) || isAWSErr(err, s3.ErrCodeNoSuchBucket, "") {
		log.Printf("[WARN] S3 Bucket (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err != nil && !isAWSErrRequestFailureStatusCode(err, 501) {
		return diag.FromErr(fmt.Errorf("error reading S3 Bucket (%s): %s", d.Id(), err))
	}

	// In the import case, we won't have this
	if _, ok := d.GetOk("bucket"); !ok {
		err := d.Set("bucket", d.Id())
		if err != nil {
			return nil
		}
	}

	bucketDomainName := d.Set("bucket_domain_name", meta.(*AWSClient).PartitionHostname(fmt.Sprintf("%s.s3", d.Get("bucket").(string))))
	if bucketDomainName != nil {
		return nil
	}

	// Add the region as an attribute
	discoveredRegion, err := retryOnAwsCode("NotFound", func() (interface{}, error) {
		return s3manager.GetBucketRegionWithClient(ctx, s3conn, d.Id(), func(r *request.Request) {
			// By default, GetBucketRegion forces virtual host addressing, which
			// is not compatible with many non-AWS implementations. Instead, pass
			// the provider s3_force_path_style configuration, which defaults to
			// false, but allows override.
			r.Config.S3ForcePathStyle = s3conn.Config.S3ForcePathStyle
		})
	})
	if err != nil && !isAWSErrRequestFailureStatusCode(err, 501) {
		return diag.FromErr(fmt.Errorf("error getting S3 Bucket location: %s", err))
	}

	region := discoveredRegion.(string)
	if err := d.Set("region", region); err != nil {
		return diag.FromErr(err)
	}

	// Add the bucket_regional_domain_name as an attribute
	regionalEndpoint, err := BucketRegionalDomainName(d.Get("bucket").(string), region)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("bucket_regional_domain_name", regionalEndpoint) //regionalEndpoint
	if err != nil {
		return nil
	}

	Arn := arn.ARN{
		Partition: meta.(*AWSClient).partition,
		Service:   "s3",
		Resource:  d.Id(),
	}.String()
	err = d.Set("arn", Arn)
	if err != nil {
		return nil
	}

	return nil
}

func resourceAwsS3BucketDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	s3conn := meta.(*AWSClient).s3conn

	log.Printf("[DEBUG] S3 Delete Bucket: %s", d.Id())
	_, err := s3conn.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(d.Id()),
	})

	if isAWSErr(err, s3.ErrCodeNoSuchBucket, "") {
		return nil
	}

	if isAWSErr(err, "BucketNotEmpty", "") {
		if d.Get("force_destroy").(bool) {
			// Use a S3 service client that can handle multiple slashes in URIs.
			// While aws_s3_bucket_object resources cannot create these object
			// keys, other AWS services and applications using the S3 Bucket can.
			s3conn = meta.(*AWSClient).s3connUriCleaningDisabled

			// bucket may have things delete them
			log.Printf("[DEBUG] S3 Bucket attempting to forceDestroy %+v", err)

			// Delete everything including locked objects.
			// Don't ignore any object errors, or we could recurse infinitely.
			err = deleteAllS3ObjectVersions(s3conn, d.Id(), "", false, false)

			if err != nil {
				return diag.FromErr(fmt.Errorf("error S3 Bucket force_destroy: %s", err))
			}

			// this line recurses until all objects are deleted or an error is returned
			return resourceAwsS3BucketDelete(ctx, d, meta)
		}
	}

	if err != nil {
		return diag.FromErr(fmt.Errorf("error deleting S3 Bucket (%s): %s", d.Id(), err))
	}

	return nil
}

func BucketRegionalDomainName(bucket string, region string) (string, error) {
	// Return a default AWS Commercial domain name if no region is provided
	// Otherwise EndpointFor() will return BUCKET.s3..amazonaws.com
	regionalBaseDomain := ""
	if region == "" {
		return fmt.Sprintf("%s.%s", bucket, regionalBaseDomain), nil //lintignore:AWSR001
	}
	endpoint, err := rabata_endpoints.RabataEndpoint(region)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%s", bucket, strings.TrimPrefix(endpoint, "https://")), nil
}

func normalizeRegion(region string) string {
	// Default to us-east-1 if the bucket doesn't have a region:
	// http://docs.aws.amazon.com/AmazonS3/latest/API/RESTBucketGETlocation.html
	if region == "" {
		region = "us-east-1"
	}

	return region
}

// validateS3BucketName validates any S3 bucket name that is not inside the us-east-1 region.
// Buckets outside of this region have to be DNS-compliant. After the same restrictions are
// applied to buckets in the us-east-1 region, this function can be refactored as a SchemaValidateFunc
func validateS3BucketName(value string, region string) error {
	if region != "us-east-1" {
		if (len(value) < 3) || (len(value) > 63) {
			return fmt.Errorf("%q must contain from 3 to 63 characters", value)
		}
		if !regexp.MustCompile(`^[0-9a-z-.]+$`).MatchString(value) {
			return fmt.Errorf("only lowercase alphanumeric characters and hyphens allowed in %q", value)
		}
		if regexp.MustCompile(`^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$`).MatchString(value) {
			return fmt.Errorf("%q must not be formatted as an IP address", value)
		}
		if strings.HasPrefix(value, `.`) {
			return fmt.Errorf("%q cannot start with a period", value)
		}
		if strings.HasSuffix(value, `.`) {
			return fmt.Errorf("%q cannot end with a period", value)
		}
		if strings.Contains(value, `..`) {
			return fmt.Errorf("%q can be only one period between labels", value)
		}
	} else {
		if len(value) > 255 {
			return fmt.Errorf("%q must contain less than 256 characters", value)
		}
		if !regexp.MustCompile(`^[0-9a-zA-Z-._]+$`).MatchString(value) {
			return fmt.Errorf("only alphanumeric characters, hyphens, periods, and underscores allowed in %q", value)
		}
	}
	return nil
}
