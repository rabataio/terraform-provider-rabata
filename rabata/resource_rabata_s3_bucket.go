package rabata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/rabataio/terraform-provider-rabata/rabata/internal/hashcode"
)

const s3BucketCreationTimeout = 2 * time.Minute

func resourceRabataS3Bucket() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceRabataS3BucketCreate,
		ReadContext:   resourceRabataS3BucketRead,
		UpdateContext: resourceRabataS3BucketUpdate,
		DeleteContext: resourceRabataS3BucketDelete,
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
				ValidateFunc:  validation.StringLenBetween(0, 63), //nolint:mnd
			},

			"bucket_prefix": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"bucket"},
				ValidateFunc:  validation.StringLenBetween(0, 63-id.UniqueIDSuffixLength), //nolint:mnd
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

			"acl": {
				Type:          schema.TypeString,
				Default:       "private",
				Optional:      true,
				ConflictsWith: []string{"grant"},
			},

			"grant": {
				Type:          schema.TypeSet,
				Optional:      true,
				Set:           grantHash,
				ConflictsWith: []string{"acl"},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"type": {
							Type:     schema.TypeString,
							Required: true,
							ValidateFunc: validation.StringInSlice([]string{
								s3.TypeCanonicalUser,
								s3.TypeGroup,
							}, false),
						},
						"uri": {
							Type:     schema.TypeString,
							Optional: true,
						},

						"permissions": {
							Type:     schema.TypeSet,
							Required: true,
							Set:      schema.HashString,
							Elem: &schema.Schema{
								Type: schema.TypeString,
								ValidateFunc: validation.StringInSlice([]string{
									s3.PermissionFullControl,
									s3.PermissionRead,
									s3.PermissionReadAcp,
									s3.PermissionWrite,
									s3.PermissionWriteAcp,
								}, false),
							},
						},
					},
				},
			},

			"region": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"force_destroy": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
	}
}

func resourceRabataS3BucketCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	awsClient := meta.(*AWSClient) //nolint:forcetypeassert
	s3conn := awsClient.s3conn

	// Get the bucket and acl
	var bucket string
	if v, ok := d.GetOk("bucket"); ok {
		bucket = v.(string) //nolint:forcetypeassert
	} else if v, ok := d.GetOk("bucket_prefix"); ok {
		bucket = id.PrefixedUniqueId(v.(string)) //nolint:forcetypeassert
	} else {
		bucket = id.UniqueId()
	}

	d.Set("bucket", bucket) //nolint:errcheck

	log.Printf("[DEBUG] S3 bucket create: %s", bucket)

	req := &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}

	if acl, ok := d.GetOk("acl"); ok {
		acl := acl.(string) //nolint:forcetypeassert
		req.ACL = aws.String(acl)
		log.Printf("[DEBUG] S3 bucket %s has canned ACL %s", bucket, acl)
	}

	awsRegion := awsClient.region
	log.Printf("[DEBUG] S3 bucket create: %s, using region: %s", bucket, awsRegion)

	// Special case us-east-1 region and do not set the LocationConstraint.
	// See "Request Elements: http://docs.aws.amazon.com/AmazonS3/latest/API/RESTBucketPUT.html
	if awsRegion != "us-east-1" {
		req.CreateBucketConfiguration = &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(awsRegion),
		}
	}

	if err := validateS3BucketName(bucket); err != nil {
		return diag.Errorf("error validating S3 bucket name: %s", err)
	}

	err := retry.RetryContext(ctx, 5*time.Minute, func() *retry.RetryError { //nolint:mnd
		log.Printf("[DEBUG] Trying to create new S3 bucket: %q", bucket)

		_, err := s3conn.CreateBucketWithContext(ctx, req)

		var awsErr awserr.Error

		if errors.As(err, &awsErr) {
			if awsErr.Code() == "OperationAborted" {
				log.Printf("[WARN] Got an error while trying to create S3 bucket %s: %s", bucket, err)

				return retry.RetryableError(
					fmt.Errorf("error creating S3 bucket %s, retrying: %w", bucket, err))
			}
		}

		if err != nil {
			return retry.NonRetryableError(err)
		}

		return nil
	})

	if isResourceTimeoutError(err) {
		_, err = s3conn.CreateBucketWithContext(ctx, req)
	}

	if err != nil {
		return diag.Errorf("error creating S3 bucket: %s", err)
	}

	// Assign the bucket name as the resource ID
	d.SetId(bucket)

	return resourceRabataS3BucketUpdate(ctx, d, meta)
}

func resourceRabataS3BucketUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	s3conn := meta.(*AWSClient).s3conn //nolint:forcetypeassert

	if d.HasChange("acl") && !d.IsNewResource() {
		if err := resourceRabataS3BucketACLUpdate(ctx, s3conn, d); err != nil {
			return diag.FromErr(err)
		}
	}

	if d.HasChange("grant") {
		if err := resourceRabataS3BucketGrantsUpdate(ctx, s3conn, d); err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceRabataS3BucketRead(ctx, d, meta)
}

func resourceRabataS3BucketRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	awsClient := meta.(*AWSClient) //nolint:forcetypeassert
	s3conn := awsClient.s3conn

	input := &s3.HeadBucketInput{
		Bucket: aws.String(d.Id()),
	}

	err := retry.RetryContext(ctx, s3BucketCreationTimeout, func() *retry.RetryError {
		_, err := s3conn.HeadBucketWithContext(ctx, input)

		if d.IsNewResource() && isAWSErrRequestFailureStatusCode(err, http.StatusNotFound) {
			return retry.RetryableError(err)
		}

		if d.IsNewResource() && isAWSErr(err, s3.ErrCodeNoSuchBucket, "") {
			return retry.RetryableError(err)
		}

		if err != nil {
			return retry.NonRetryableError(err)
		}

		return nil
	})

	if isResourceTimeoutError(err) {
		_, err = s3conn.HeadBucketWithContext(ctx, input)
	}

	if isAWSErrRequestFailureStatusCode(err, http.StatusNotFound) || isAWSErr(err, s3.ErrCodeNoSuchBucket, "") {
		log.Printf("[WARN] S3 Bucket (%s) not found, removing from state", d.Id())
		d.SetId("")

		return nil
	}

	if err != nil {
		return diag.Errorf("error reading S3 Bucket (%s): %s", d.Id(), err)
	}

	// In the import case, we won't have this
	if _, ok := d.GetOk("bucket"); !ok {
		d.Set("bucket", d.Id()) //nolint:errcheck
	}

	bucketDomainName := awsClient.PartitionHostname(d.Get("bucket").(string) + ".s3") //nolint:forcetypeassert

	d.Set("bucket_domain_name", bucketDomainName) //nolint:errcheck

	// Read the Grant ACL. Reset if `acl` (canned ACL) is set.
	if acl, ok := d.GetOk("acl"); ok && acl.(string) != "private" { //nolint:forcetypeassert
		if err := d.Set("grant", nil); err != nil {
			return diag.Errorf("error resetting grant %s", err)
		}
	} else {
		apResponse, err := retryOnAWSCode(ctx, "NoSuchBucket", func() (any, error) {
			return s3conn.GetBucketAclWithContext(ctx, &s3.GetBucketAclInput{
				Bucket: aws.String(d.Id()),
			})
		})
		if err != nil {
			return diag.Errorf("error getting S3 Bucket (%s) ACL: %s", d.Id(), err)
		}

		log.Printf("[DEBUG] S3 bucket: %s, read ACL grants policy: %+v", d.Id(), apResponse)

		grants := flattenGrants(apResponse.(*s3.GetBucketAclOutput)) //nolint:forcetypeassert
		if err := d.Set("grant", schema.NewSet(grantHash, grants)); err != nil {
			return diag.Errorf("error setting grant %s", err)
		}
	}

	// Add the region as an attribute
	discoveredRegion, err := retryOnAWSCode(ctx, "NotFound", func() (any, error) {
		return s3manager.GetBucketRegionWithClient(ctx, s3conn, d.Id(), func(r *request.Request) {
			// By default, GetBucketRegion forces virtual host addressing, which
			// is not compatible with many non-AWS implementations. Instead, pass
			// the provider s3_force_path_style configuration, which defaults to
			// false, but allows override.
			r.Config.S3ForcePathStyle = s3conn.Config.S3ForcePathStyle
		})
	})
	if err != nil {
		return diag.Errorf("error getting S3 Bucket location: %s", err)
	}

	region := discoveredRegion.(string) //nolint:forcetypeassert
	if err := d.Set("region", region); err != nil {
		return diag.FromErr(err)
	}

	d.Set("bucket_regional_domain_name", bucketDomainName) //nolint:errcheck

	a := arn.ARN{
		Partition: "aws",
		Service:   "s3",
		Resource:  d.Id(),
	}.String()
	d.Set("arn", a) //nolint:errcheck

	return nil
}

func resourceRabataS3BucketDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	awsClient := meta.(*AWSClient) //nolint:forcetypeassert
	s3conn := awsClient.s3conn

	log.Printf("[DEBUG] S3 Delete Bucket: %s", d.Id())
	_, err := s3conn.DeleteBucketWithContext(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(d.Id()),
	})

	if isAWSErr(err, s3.ErrCodeNoSuchBucket, "") {
		return nil
	}

	if isAWSErr(err, "BucketNotEmpty", "") {
		if d.Get("force_destroy").(bool) { //nolint:forcetypeassert
			// Use a S3 service client that can handle multiple slashes in URIs.
			// While rabata_s3_bucket_object resources cannot create these object
			// keys, other AWS services and applications using the S3 Bucket can.
			s3conn = awsClient.s3connURICleaningDisabled

			// bucket may have things delete them
			log.Printf("[DEBUG] S3 Bucket attempting to forceDestroy %+v", err)

			// Delete everything including locked objects.
			// Don't ignore any object errors or we could recurse infinitely.
			err = deleteAllS3Objects(ctx, s3conn, d.Id(), "", false, false)
			if err != nil {
				return diag.Errorf("error S3 Bucket force_destroy: %s", err)
			}

			// this line recurses until all objects are deleted or an error is returned
			return resourceRabataS3BucketDelete(ctx, d, meta)
		}
	}

	if err != nil {
		return diag.Errorf("error deleting S3 Bucket (%s): %s", d.Id(), err)
	}

	return nil
}

func resourceRabataS3BucketGrantsUpdate(ctx context.Context, s3conn *s3.S3, d *schema.ResourceData) error {
	bucket := d.Get("bucket").(string)               //nolint:forcetypeassert
	rawGrants := d.Get("grant").(*schema.Set).List() //nolint:forcetypeassert

	if len(rawGrants) == 0 { //nolint:nestif
		log.Printf("[DEBUG] S3 bucket: %s, Grants fallback to canned ACL", bucket)

		if err := resourceRabataS3BucketACLUpdate(ctx, s3conn, d); err != nil {
			return fmt.Errorf("error fallback to canned ACL, %w", err)
		}
	} else {
		apResponse, err := retryOnAWSCode(ctx, "NoSuchBucket", func() (any, error) {
			return s3conn.GetBucketAclWithContext(ctx, &s3.GetBucketAclInput{
				Bucket: aws.String(d.Id()),
			})
		})
		if err != nil {
			return fmt.Errorf("error getting S3 Bucket (%s) ACL: %w", d.Id(), err)
		}

		ap := apResponse.(*s3.GetBucketAclOutput) //nolint:forcetypeassert
		log.Printf("[DEBUG] S3 bucket: %s, read ACL grants policy: %+v", d.Id(), ap)

		grants := make([]*s3.Grant, 0, len(rawGrants))

		for _, rawGrant := range rawGrants {
			log.Printf("[DEBUG] S3 bucket: %s, put grant: %#v", bucket, rawGrant)
			grantMap := rawGrant.(map[string]any) //nolint:forcetypeassert

			for _, rawPermission := range grantMap["permissions"].(*schema.Set).List() { //nolint:forcetypeassert
				ge := &s3.Grantee{}
				if i, ok := grantMap["id"].(string); ok && i != "" {
					ge.SetID(i)
				}

				if t, ok := grantMap["type"].(string); ok && t != "" {
					ge.SetType(t)
				}

				if u, ok := grantMap["uri"].(string); ok && u != "" {
					ge.SetURI(u)
				}

				//nolint:forcetypeassert
				g := &s3.Grant{
					Grantee:    ge,
					Permission: aws.String(rawPermission.(string)),
				}
				grants = append(grants, g)
			}
		}

		grantsInput := &s3.PutBucketAclInput{
			Bucket: aws.String(bucket),
			AccessControlPolicy: &s3.AccessControlPolicy{
				Grants: grants,
				Owner:  ap.Owner,
			},
		}

		log.Printf("[DEBUG] S3 bucket: %s, put Grants: %#v", bucket, grantsInput)

		_, err = retryOnAWSCode(ctx, "NoSuchBucket", func() (any, error) {
			return s3conn.PutBucketAclWithContext(ctx, grantsInput)
		})
		if err != nil {
			return fmt.Errorf("error putting S3 Grants: %w", err)
		}
	}

	return nil
}

func resourceRabataS3BucketACLUpdate(ctx context.Context, s3conn *s3.S3, d *schema.ResourceData) error {
	acl := d.Get("acl").(string)       //nolint:forcetypeassert
	bucket := d.Get("bucket").(string) //nolint:forcetypeassert

	i := &s3.PutBucketAclInput{
		Bucket: aws.String(bucket),
		ACL:    aws.String(acl),
	}
	log.Printf("[DEBUG] S3 put bucket ACL: %#v", i)

	_, err := retryOnAWSCode(ctx, s3.ErrCodeNoSuchBucket, func() (any, error) {
		return s3conn.PutBucketAclWithContext(ctx, i)
	})
	if err != nil {
		return fmt.Errorf("error putting S3 ACL: %w", err)
	}

	return nil
}

// validateS3BucketName validates any S3 bucket name.
func validateS3BucketName(value string) error {
	if (len(value) < 3) || (len(value) > 63) { //nolint:mnd
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

	return nil
}

func grantHash(v any) int {
	var buf bytes.Buffer

	m, ok := v.(map[string]any)
	if !ok {
		return 0
	}

	if v, ok := m["id"]; ok {
		buf.WriteString(v.(string) + "-") //nolint:forcetypeassert
	}

	if v, ok := m["type"]; ok {
		buf.WriteString(v.(string) + "-") //nolint:forcetypeassert
	}

	if v, ok := m["uri"]; ok {
		buf.WriteString(v.(string) + "-") //nolint:forcetypeassert
	}

	if p, ok := m["permissions"]; ok {
		buf.WriteString(fmt.Sprintf("%v-", p.(*schema.Set).List())) //nolint:forcetypeassert
	}

	return hashcode.String(buf.String())
}

func flattenGrants(ap *s3.GetBucketAclOutput) []any {
	// if ACL grants contains bucket owner FULL_CONTROL only - it is default "private" acl
	if len(ap.Grants) == 1 && aws.StringValue(ap.Grants[0].Grantee.ID) == aws.StringValue(ap.Owner.ID) &&
		aws.StringValue(ap.Grants[0].Permission) == s3.PermissionFullControl {
		return nil
	}

	getGrant := func(grants []any, grantee map[string]any) (any, bool) {
		for _, pg := range grants {
			pgt := pg.(map[string]any) //nolint:forcetypeassert
			if pgt["type"] == grantee["type"] && pgt["id"] == grantee["id"] && pgt["uri"] == grantee["uri"] &&
				pgt["permissions"].(*schema.Set).Len() > 0 { //nolint:forcetypeassert
				return pg, true
			}
		}

		return nil, false
	}

	grants := make([]any, 0, len(ap.Grants))

	for _, granteeObject := range ap.Grants {
		grantee := make(map[string]any)
		grantee["type"] = aws.StringValue(granteeObject.Grantee.Type)

		if granteeObject.Grantee.ID != nil {
			grantee["id"] = aws.StringValue(granteeObject.Grantee.ID)
		}

		if granteeObject.Grantee.URI != nil {
			grantee["uri"] = aws.StringValue(granteeObject.Grantee.URI)
		}

		if pg, ok := getGrant(grants, grantee); ok {
			permissionSet := pg.(map[string]any)["permissions"].(*schema.Set) //nolint:forcetypeassert
			permissionSet.Add(aws.StringValue(granteeObject.Permission))
		} else {
			grantee["permissions"] = schema.NewSet(schema.HashString, []any{aws.StringValue(granteeObject.Permission)})
			grants = append(grants, grantee)
		}
	}

	return grants
}
