package rabata

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/mitchellh/go-homedir"
)

func resourceRabataS3BucketObject() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceRabataS3BucketObjectCreate,
		ReadContext:   resourceRabataS3BucketObjectRead,
		UpdateContext: resourceRabataS3BucketObjectUpdate,
		DeleteContext: resourceRabataS3BucketObjectDelete,

		CustomizeDiff: resourceRabataS3BucketObjectCustomizeDiff,

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.NoZeroValues,
			},

			"key": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.NoZeroValues,
			},

			"acl": {
				Type:     schema.TypeString,
				Default:  s3.ObjectCannedACLPrivate,
				Optional: true,
				ValidateFunc: validation.StringInSlice([]string{
					s3.ObjectCannedACLPrivate,
					s3.ObjectCannedACLPublicRead,
					s3.ObjectCannedACLPublicReadWrite,
					s3.ObjectCannedACLAuthenticatedRead,
					s3.ObjectCannedACLAwsExecRead,
					s3.ObjectCannedACLBucketOwnerRead,
					s3.ObjectCannedACLBucketOwnerFullControl,
				}, false),
			},

			"cache_control": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"content_disposition": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"content_encoding": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"content_language": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"metadata": {
				Type:         schema.TypeMap,
				ValidateFunc: validateMetadataIsLowerCase,
				Optional:     true,
				Elem:         &schema.Schema{Type: schema.TypeString},
			},

			"content_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"source": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"content", "content_base64"},
			},

			"content": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"source", "content_base64"},
			},

			"content_base64": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"source", "content"},
			},

			"storage_class": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ValidateFunc: validation.StringInSlice([]string{
					s3.ObjectStorageClassStandard,
					s3.ObjectStorageClassReducedRedundancy,
					s3.ObjectStorageClassGlacier,
					s3.ObjectStorageClassStandardIa,
					s3.ObjectStorageClassOnezoneIa,
					s3.ObjectStorageClassIntelligentTiering,
					s3.ObjectStorageClassDeepArchive,
				}, false),
			},

			"etag": {
				Type: schema.TypeString,
				// This will conflict with SSE-C and multi-part upload
				// if/when it's actually implemented. The Etag then won't match raw-file MD5.
				// See http://docs.aws.amazon.com/AmazonS3/latest/API/RESTCommonResponseHeaders.html
				Optional: true,
				Computed: true,
			},

			"version_id": {
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

func resourceRabataS3BucketObjectPut(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	s3conn := meta.(*AWSClient).s3conn //nolint:forcetypeassert

	var body io.ReadSeeker

	if v, ok := d.GetOk("source"); ok {
		source := v.(string) //nolint:forcetypeassert

		path, err := homedir.Expand(source)
		if err != nil {
			return diag.Errorf("Error expanding homedir in source (%s): %s", source, err)
		}

		file, err := os.Open(path)
		if err != nil {
			return diag.Errorf("Error opening S3 bucket object source (%s): %s", path, err)
		}

		body = file

		defer func() {
			err := file.Close()
			if err != nil {
				log.Printf("[WARN] Error closing S3 bucket object source (%s): %s", path, err)
			}
		}()
	} else if v, ok := d.GetOk("content"); ok {
		content := v.(string) //nolint:forcetypeassert
		body = bytes.NewReader([]byte(content))
	} else if v, ok := d.GetOk("content_base64"); ok {
		content := v.(string) //nolint:forcetypeassert
		// We can't do streaming decoding here (with base64.NewDecoder) because
		// the AWS SDK requires an io.ReadSeeker but a base64 decoder can't seek.
		contentRaw, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return diag.Errorf("error decoding content_base64: %s", err)
		}

		body = bytes.NewReader(contentRaw)
	}

	bucket := d.Get("bucket").(string) //nolint:forcetypeassert
	key := d.Get("key").(string)       //nolint:forcetypeassert

	//nolint:forcetypeassert
	putInput := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		ACL:    aws.String(d.Get("acl").(string)),
		Body:   body,
	}

	if v, ok := d.GetOk("storage_class"); ok {
		putInput.StorageClass = aws.String(v.(string)) //nolint:forcetypeassert
	}

	if v, ok := d.GetOk("cache_control"); ok {
		putInput.CacheControl = aws.String(v.(string)) //nolint:forcetypeassert
	}

	if v, ok := d.GetOk("content_type"); ok {
		putInput.ContentType = aws.String(v.(string)) //nolint:forcetypeassert
	}

	if v, ok := d.GetOk("metadata"); ok {
		putInput.Metadata = stringMapToPointers(v.(map[string]any)) //nolint:forcetypeassert
	}

	if v, ok := d.GetOk("content_encoding"); ok {
		putInput.ContentEncoding = aws.String(v.(string)) //nolint:forcetypeassert
	}

	if v, ok := d.GetOk("content_language"); ok {
		putInput.ContentLanguage = aws.String(v.(string)) //nolint:forcetypeassert
	}

	if v, ok := d.GetOk("content_disposition"); ok {
		putInput.ContentDisposition = aws.String(v.(string)) //nolint:forcetypeassert
	}

	if _, err := s3conn.PutObjectWithContext(ctx, putInput); err != nil {
		return diag.Errorf("Error putting object in S3 bucket (%s): %s", bucket, err)
	}

	d.SetId(key)

	return resourceRabataS3BucketObjectRead(ctx, d, meta)
}

func resourceRabataS3BucketObjectCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	return resourceRabataS3BucketObjectPut(ctx, d, meta)
}

func resourceRabataS3BucketObjectRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	s3conn := meta.(*AWSClient).s3conn //nolint:forcetypeassert

	bucket := d.Get("bucket").(string) //nolint:forcetypeassert
	key := d.Get("key").(string)       //nolint:forcetypeassert

	resp, err := s3conn.HeadObjectWithContext(
		ctx,
		&s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		},
	)
	if err != nil {
		var awsErr awserr.RequestFailure
		// If S3 returns a 404 Request Failure, mark the object as destroyed
		if errors.As(err, &awsErr) && awsErr.StatusCode() == http.StatusNotFound {
			d.SetId("")
			log.Printf("[WARN] Error Reading Object (%s), object not found (HTTP status 404)", key)

			return nil
		}

		return diag.FromErr(err)
	}

	log.Printf("[DEBUG] Reading S3 Bucket Object meta: %s", resp)

	d.Set("cache_control", resp.CacheControl)             //nolint:errcheck
	d.Set("content_disposition", resp.ContentDisposition) //nolint:errcheck
	d.Set("content_encoding", resp.ContentEncoding)       //nolint:errcheck
	d.Set("content_language", resp.ContentLanguage)       //nolint:errcheck
	d.Set("content_type", resp.ContentType)               //nolint:errcheck
	metadata := pointersMapToStringList(resp.Metadata)

	// AWS Go SDK capitalizes metadata, this is a workaround. https://github.com/aws/aws-sdk-go/issues/445
	for k, v := range metadata {
		delete(metadata, k)
		metadata[strings.ToLower(k)] = v
	}

	if err := d.Set("metadata", metadata); err != nil {
		return diag.Errorf("error setting metadata: %s", err)
	}

	d.Set("version_id", resp.VersionId) //nolint:errcheck

	// See https://forums.aws.amazon.com/thread.jspa?threadID=44003
	d.Set("etag", strings.Trim(aws.StringValue(resp.ETag), `"`)) //nolint:errcheck

	// The "STANDARD" (which is also the default) storage
	// class when set would not be included in the results.
	storageClass := s3.StorageClassStandard
	if resp.StorageClass != nil {
		storageClass = *resp.StorageClass
	}

	d.Set("storage_class", storageClass) //nolint:errcheck

	return nil
}

func resourceRabataS3BucketObjectUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	// Changes to any of these attributes requires creation of a new object version (if bucket is versioned):
	for _, key := range []string{
		"cache_control",
		"content_base64",
		"content_disposition",
		"content_encoding",
		"content_language",
		"content_type",
		"content",
		"etag",
		"metadata",
		"source",
		"storage_class",
	} {
		if d.HasChange(key) {
			return resourceRabataS3BucketObjectPut(ctx, d, meta)
		}
	}

	conn := meta.(*AWSClient).s3conn //nolint:forcetypeassert

	bucket := d.Get("bucket").(string) //nolint:forcetypeassert
	key := d.Get("key").(string)       //nolint:forcetypeassert

	if d.HasChange("acl") {
		_, err := conn.PutObjectAclWithContext(
			ctx,
			&s3.PutObjectAclInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(key),
				ACL:    aws.String(d.Get("acl").(string)),
			},
		)
		if err != nil {
			return diag.Errorf("error putting S3 object ACL: %s", err)
		}
	}

	return resourceRabataS3BucketObjectRead(ctx, d, meta)
}

func resourceRabataS3BucketObjectDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	s3conn := meta.(*AWSClient).s3conn //nolint:forcetypeassert

	bucket := d.Get("bucket").(string) //nolint:forcetypeassert
	key := d.Get("key").(string)       //nolint:forcetypeassert
	// We are effectively ignoring any leading '/' in the key name as aws.Config.DisableRestProtocolURICleaning is false
	key = strings.TrimPrefix(key, "/")

	var err error
	if _, ok := d.GetOk("version_id"); ok {
		//nolint:forcetypeassert
		err = deleteAllS3Objects(
			ctx,
			s3conn,
			bucket,
			key,
			d.Get("force_destroy").(bool),
			false,
		)
	} else {
		err = deleteS3ObjectVersion(ctx, s3conn, bucket, key, "", false)
	}

	if err != nil {
		return diag.Errorf("error deleting S3 Bucket (%s) Object (%s): %s", bucket, key, err)
	}

	return nil
}

func validateMetadataIsLowerCase(v any, _ string) ([]string, []error) {
	value := v.(map[string]any) //nolint:forcetypeassert

	var errs []error

	for k := range value {
		if k != strings.ToLower(k) {
			errs = append(errs, fmt.Errorf(
				"metadata must be lowercase only. Offending key: %q", k))
		}
	}

	return nil, errs
}

func resourceRabataS3BucketObjectCustomizeDiff(_ context.Context, d *schema.ResourceDiff, _ any) error {
	if d.HasChange("etag") {
		d.SetNewComputed("version_id") //nolint:errcheck
	}

	return nil
}

// deleteAllS3Objects deletes key from an S3 bucket.
// If key is empty then all objects are deleted.
// Set force to true to override any S3 object lock protections on object lock enabled buckets.
func deleteAllS3Objects(
	ctx context.Context,
	conn *s3.S3,
	bucketName, key string,
	force, ignoreObjectErrors bool,
) error {
	// TODO: Replace to ListObjectVersionsInput when implement.
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	}
	if key != "" {
		input.Prefix = aws.String(key)
	}

	var lastErr error

	err := conn.ListObjectsV2PagesWithContext(
		ctx,
		input,
		func(page *s3.ListObjectsV2Output, lastPage bool) bool {
			if page == nil {
				return !lastPage
			}

			for _, object := range page.Contents {
				objectKey := aws.StringValue(object.Key)

				if key != "" && key != objectKey {
					continue
				}

				err := deleteS3ObjectVersion(ctx, conn, bucketName, objectKey, "", force)
				if err != nil {
					lastErr = err
				}
			}

			return !lastPage
		},
	)

	if isAWSErr(err, s3.ErrCodeNoSuchBucket, "") {
		err = nil
	}

	if err != nil {
		return err
	}

	if lastErr != nil {
		if !ignoreObjectErrors {
			return fmt.Errorf("error deleting at least one object version, last error: %w", lastErr)
		}

		lastErr = nil
	}

	if isAWSErr(err, s3.ErrCodeNoSuchBucket, "") {
		err = nil
	}

	if err != nil {
		return err
	}

	if lastErr != nil {
		if !ignoreObjectErrors {
			return fmt.Errorf("error deleting at least one object delete marker, last error: %w", lastErr)
		}

		lastErr = nil
	}

	return nil
}

// deleteS3ObjectVersion deletes a specific bucket object version.
// Set force to true to override any S3 object lock protections.
func deleteS3ObjectVersion(ctx context.Context, conn *s3.S3, b, k, v string, force bool) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(b),
		Key:    aws.String(k),
	}

	if v != "" {
		input.VersionId = aws.String(v)
	}

	if force {
		input.BypassGovernanceRetention = aws.Bool(true)
	}

	log.Printf("[INFO] Deleting S3 Bucket (%s) Object (%s) Version: %s", b, k, v)

	_, err := conn.DeleteObjectWithContext(ctx, input)
	if err != nil {
		log.Printf("[WARN] Error deleting S3 Bucket (%s) Object (%s) Version (%s): %s", b, k, v, err)
	}

	if isAWSErr(err, s3.ErrCodeNoSuchBucket, "") || isAWSErr(err, s3.ErrCodeNoSuchKey, "") {
		return nil
	}

	return err
}
