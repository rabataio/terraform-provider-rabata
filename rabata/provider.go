package rabata

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider returns a *schema.Provider.
func Provider() *schema.Provider {
	// TODO: Move the validation to this, requires conditional schemas
	// TODO: Move the configuration to this, requires validation
	// The actual provider
	provider := &schema.Provider{
		Schema: map[string]*schema.Schema{
			"access_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: descriptions["access_key"],
				DefaultFunc: schema.EnvDefaultFunc("RABATA_ACCESS_KEY", nil),
			},

			"secret_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: descriptions["secret_key"],
				DefaultFunc: schema.EnvDefaultFunc("RABATA_SECRET_KEY", nil),
			},

			"profile": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: descriptions["profile"],
			},

			"shared_credentials_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: descriptions["shared_credentials_file"],
			},

			"region": {
				Type:         schema.TypeString,
				Required:     true,
				DefaultFunc:  schema.EnvDefaultFunc("RABATA_REGION", nil),
				Description:  descriptions["region"],
				InputDefault: "us-east-1",
			},

			"max_retries": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     25, //nolint:mnd
				Description: descriptions["max_retries"],
			},

			"endpoints": endpointsSchema(),

			"insecure": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: descriptions["insecure"],
			},

			"s3_force_path_style": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: descriptions["s3_force_path_style"],
			},
		},

		DataSourcesMap: map[string]*schema.Resource{
			"rabata_s3_bucket":         dataSourceRabataS3Bucket(),
			"rabata_s3_bucket_object":  dataSourceRabataS3BucketObject(),
			"rabata_s3_bucket_objects": dataSourceRabataS3BucketObjects(),
		},

		ResourcesMap: map[string]*schema.Resource{
			"rabata_s3_bucket":        resourceRabataS3Bucket(),
			"rabata_s3_bucket_object": resourceRabataS3BucketObject(),
		},
	}

	provider.ConfigureContextFunc = func(ctx context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
		terraformVersion := provider.TerraformVersion
		if terraformVersion == "" {
			// Terraform 0.12 introduced this field to the protocol
			// We can therefore assume that if it's missing it's 0.10 or 0.11
			terraformVersion = "0.11+compatible"
		}

		return providerConfigure(d, terraformVersion)
	}

	return provider
}

var (
	descriptions         map[string]string
	endpointServiceNames []string
)

func init() {
	descriptions = map[string]string{
		"region": "The region where Rabata operations will take place. Examples\n" +
			"are eu-west-1, us-east-1, etc.",

		"access_key": "The access key for API operations. You can retrieve this\n" +
			"from the 'Security & Credentials' section of the Rabata.io.",

		"secret_key": "The secret key for API operations. You can retrieve this\n" +
			"from the 'Security & Credentials' section of the Rabata.io.",

		"profile": "The profile for API operations. If not set, the default profile\n" +
			"created with `aws configure` will be used.",

		"shared_credentials_file": "The path to the shared credentials file. If not set\n" +
			"this defaults to ~/.aws/credentials.",

		"max_retries": "The maximum number of times an Rabata API request is\n" +
			"being executed. If the API request still fails, an error is\n" +
			"thrown.",

		"insecure": "Explicitly allow the provider to perform \"insecure\" SSL requests. If omitted," +
			"default value is `false`",

		"s3_force_path_style": "Set this to true to force the request to use path-style addressing,\n" +
			"i.e., http://s3.eu-west-1.rabata.io/BUCKET/KEY. By default, the S3 client will\n" +
			"use virtual hosted bucket addressing when possible\n" +
			"(http://BUCKET.s3.eu-west-1.rabata.io/KEY). Specific to the S3 service.",
	}

	endpointServiceNames = []string{
		"s3",
	}
}

func getDNSSuffix(region string) string {
	if region == "" {
		region = "eu-west-1"
	}

	return region + ".rabata.io"
}

func providerConfigure(d *schema.ResourceData, terraformVersion string) (any, diag.Diagnostics) {
	region := d.Get("region").(string) //nolint:forcetypeassert

	//nolint:forcetypeassert
	config := Config{
		AccessKey:     d.Get("access_key").(string),
		SecretKey:     d.Get("secret_key").(string),
		Profile:       d.Get("profile").(string),
		Region:        region,
		CredsFilename: d.Get("shared_credentials_file").(string),
		Endpoints: map[string]string{
			"s3": "https://s3." + getDNSSuffix(region),
		},
		MaxRetries:       d.Get("max_retries").(int),
		Insecure:         d.Get("insecure").(bool),
		S3ForcePathStyle: d.Get("s3_force_path_style").(bool),
		terraformVersion: terraformVersion,
	}

	endpointsSet := d.Get("endpoints").(*schema.Set) //nolint:forcetypeassert

	for _, endpointsSetI := range endpointsSet.List() {
		endpoints := endpointsSetI.(map[string]any) //nolint:forcetypeassert
		for _, endpointServiceName := range endpointServiceNames {
			config.Endpoints[endpointServiceName] = endpoints[endpointServiceName].(string) //nolint:forcetypeassert
		}
	}

	client, err := config.Client()
	if err != nil {
		return nil, diag.FromErr(err)
	}

	return client, nil
}

func endpointsSchema() *schema.Schema {
	endpointsAttributes := make(map[string]*schema.Schema)

	for _, endpointServiceName := range endpointServiceNames {
		endpointsAttributes[endpointServiceName] = &schema.Schema{
			Type:        schema.TypeString,
			Optional:    true,
			Default:     "",
			Description: descriptions["endpoint"],
		}
	}

	return &schema.Schema{
		Type:     schema.TypeSet,
		Optional: true,
		Elem: &schema.Resource{
			Schema: endpointsAttributes,
		},
	}
}
