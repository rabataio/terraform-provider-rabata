package rabata

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceRabataS3BucketObject() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceRabataS3BucketObjectRead,

		Schema: map[string]*schema.Schema{
			"body": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"bucket": {
				Type:     schema.TypeString,
				Required: true,
			},
			"cache_control": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"content_disposition": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"content_encoding": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"content_language": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"content_length": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"content_type": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"etag": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"expiration": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"expires": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"key": {
				Type:     schema.TypeString,
				Required: true,
			},
			"last_modified": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"metadata": {
				Type:     schema.TypeMap,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"range": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"sse_kms_key_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"storage_class": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"version_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func dataSourceRabataS3BucketObjectRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	conn := meta.(*AWSClient).s3conn //nolint:forcetypeassert

	bucket := d.Get("bucket").(string) //nolint:forcetypeassert
	key := d.Get("key").(string)       //nolint:forcetypeassert

	input := s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	if v, ok := d.GetOk("range"); ok {
		input.Range = aws.String(v.(string)) //nolint:forcetypeassert
	}

	if v, ok := d.GetOk("version_id"); ok {
		input.VersionId = aws.String(v.(string)) //nolint:forcetypeassert
	}

	versionText := ""
	uniqueID := bucket + "/" + key

	if v, ok := d.GetOk("version_id"); ok {
		versionID := v.(string) //nolint:forcetypeassert
		versionText = fmt.Sprintf(" of version %q", versionID)
		uniqueID += "@" + versionID
	}

	log.Printf("[DEBUG] Reading S3 Bucket Object: %s", input)

	out, err := conn.HeadObjectWithContext(ctx, &input)
	if err != nil {
		return diag.Errorf("Failed getting S3 object: %s Bucket: %q Object: %q", err, bucket, key)
	}

	if out.DeleteMarker != nil && *out.DeleteMarker {
		return diag.Errorf("Requested S3 object %q%s has been deleted",
			bucket+key, versionText)
	}

	log.Printf("[DEBUG] Received S3 object: %s", out)

	d.SetId(uniqueID)

	d.Set("cache_control", out.CacheControl)             //nolint:errcheck
	d.Set("content_disposition", out.ContentDisposition) //nolint:errcheck
	d.Set("content_encoding", out.ContentEncoding)       //nolint:errcheck
	d.Set("content_language", out.ContentLanguage)       //nolint:errcheck
	d.Set("content_length", out.ContentLength)           //nolint:errcheck
	d.Set("content_type", out.ContentType)               //nolint:errcheck
	// See https://forums.aws.amazon.com/thread.jspa?threadID=44003
	d.Set("etag", strings.Trim(*out.ETag, `"`))                   //nolint:errcheck
	d.Set("expiration", out.Expiration)                           //nolint:errcheck
	d.Set("expires", out.Expires)                                 //nolint:errcheck
	d.Set("last_modified", out.LastModified.Format(time.RFC1123)) //nolint:errcheck
	d.Set("metadata", pointersMapToStringList(out.Metadata))      //nolint:errcheck
	d.Set("sse_kms_key_id", out.SSEKMSKeyId)                      //nolint:errcheck
	d.Set("version_id", out.VersionId)                            //nolint:errcheck

	// The "STANDARD" (which is also the default) storage
	// class when set would not be included in the results.
	storageClass := s3.StorageClassStandard
	if out.StorageClass != nil {
		storageClass = *out.StorageClass
	}

	d.Set("storage_class", storageClass) //nolint:errcheck

	if isContentTypeAllowed(out.ContentType) {
		input := s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		}
		if v, ok := d.GetOk("range"); ok {
			input.Range = aws.String(v.(string)) //nolint:forcetypeassert
		}

		if out.VersionId != nil {
			input.VersionId = out.VersionId
		}

		out, err := conn.GetObjectWithContext(ctx, &input)
		if err != nil {
			return diag.Errorf("Failed getting S3 object: %s", err)
		}

		buf := new(bytes.Buffer)

		bytesRead, err := buf.ReadFrom(out.Body)
		if err != nil {
			return diag.Errorf("Failed reading content of S3 object (%s): %s",
				uniqueID, err)
		}

		log.Printf("[INFO] Saving %d bytes from S3 object %s", bytesRead, uniqueID)
		d.Set("body", buf.String()) //nolint:errcheck
	} else {
		var contentType string
		if out.ContentType == nil {
			contentType = "<EMPTY>"
		} else {
			contentType = *out.ContentType
		}

		log.Printf("[INFO] Ignoring body of S3 object %s with Content-Type %q",
			uniqueID, contentType)
	}

	return nil
}

// This is to prevent potential issues w/ binary files
// and generally unprintable characters
// See https://github.com/hashicorp/terraform/pull/3858#issuecomment-156856738
func isContentTypeAllowed(contentType *string) bool {
	if contentType == nil {
		return false
	}

	allowedContentTypes := []*regexp.Regexp{
		regexp.MustCompile("^text/.+"),
		regexp.MustCompile("^application/json$"),
	}

	for _, r := range allowedContentTypes {
		if r.MatchString(*contentType) {
			return true
		}
	}

	return false
}
