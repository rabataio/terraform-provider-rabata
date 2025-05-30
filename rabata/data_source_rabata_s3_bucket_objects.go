package rabata

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const keyRequestPageSize = 1000

func dataSourceRabataS3BucketObjects() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceRabataS3BucketObjectsRead,

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:     schema.TypeString,
				Required: true,
			},
			"prefix": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"delimiter": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"encoding_type": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"max_keys": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1000, //nolint:mnd
			},
			"start_after": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"fetch_owner": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"keys": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"common_prefixes": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"owners": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func dataSourceRabataS3BucketObjectsRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	conn := meta.(*AWSClient).s3conn //nolint:forcetypeassert

	bucket := d.Get("bucket").(string) //nolint:forcetypeassert
	prefix := d.Get("prefix").(string) //nolint:forcetypeassert

	d.SetId(id.UniqueId())

	listInput := s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}

	if prefix != "" {
		listInput.Prefix = aws.String(prefix)
	}

	if s, ok := d.GetOk("delimiter"); ok {
		listInput.Delimiter = aws.String(s.(string)) //nolint:forcetypeassert
	}

	if s, ok := d.GetOk("encoding_type"); ok {
		listInput.EncodingType = aws.String(s.(string)) //nolint:forcetypeassert
	}

	// "listInput.MaxKeys" refers to max keys returned in a single request
	// (i.e., page size), not the total number of keys returned if you page
	// through the results. "maxKeys" does refer to total keys returned.
	maxKeys := int64(d.Get("max_keys").(int)) //nolint:forcetypeassert
	if maxKeys <= keyRequestPageSize {
		listInput.MaxKeys = aws.Int64(maxKeys)
	}

	if s, ok := d.GetOk("start_after"); ok {
		listInput.StartAfter = aws.String(s.(string)) //nolint:forcetypeassert
	}

	if b, ok := d.GetOk("fetch_owner"); ok {
		listInput.FetchOwner = aws.Bool(b.(bool)) //nolint:forcetypeassert
	}

	var (
		commonPrefixes []string
		keys           []string
		owners         []string
	)

	err := conn.ListObjectsV2PagesWithContext(
		ctx,
		&listInput,
		func(page *s3.ListObjectsV2Output, lastPage bool) bool {
			for _, commonPrefix := range page.CommonPrefixes {
				commonPrefixes = append(commonPrefixes, aws.StringValue(commonPrefix.Prefix))
			}

			for _, object := range page.Contents {
				keys = append(keys, aws.StringValue(object.Key))

				if object.Owner != nil {
					owners = append(owners, aws.StringValue(object.Owner.ID))
				}
			}

			maxKeys -= aws.Int64Value(page.KeyCount)

			if maxKeys <= keyRequestPageSize {
				listInput.MaxKeys = aws.Int64(maxKeys)
			}

			return !lastPage
		},
	)
	if err != nil {
		return diag.Errorf("error listing S3 Bucket (%s) Objects: %s", bucket, err)
	}

	if err := d.Set("common_prefixes", commonPrefixes); err != nil {
		return diag.Errorf("error setting common_prefixes: %s", err)
	}

	if err := d.Set("keys", keys); err != nil {
		return diag.Errorf("error setting keys: %s", err)
	}

	if err := d.Set("owners", owners); err != nil {
		return diag.Errorf("error setting owners: %s", err)
	}

	return nil
}
