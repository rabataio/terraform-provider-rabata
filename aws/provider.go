package aws

import (
	"RabataTerraformProvider/aws/internal/rabata_endpoints"
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

			"token": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: descriptions["token"],
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
				Default:     25,
				Description: descriptions["max_retries"],
			},

			"endpoints": endpointsSchema(),

			"skip_credentials_validation": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: descriptions["skip_credentials_validation"],
			},

			"skip_requesting_account_id": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: descriptions["skip_requesting_account_id"],
			},

			"skip_metadata_api_check": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: descriptions["skip_metadata_api_check"],
			},

			"s3_force_path_style": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: descriptions["s3_force_path_style"],
			},
		},

		DataSourcesMap: map[string]*schema.Resource{
			"rabata_s3_bucket":      dataSourceAwsS3Bucket(),
			"rabata_bucket_object":  dataSourceAwsS3BucketObject(),
			"rabata_bucket_objects": dataSourceAwsS3BucketObjects(),
		},

		ResourcesMap: map[string]*schema.Resource{
			"rabata_s3_bucket":     resourceAwsS3Bucket(),
			"rabata_bucket_object": resourceAwsS3BucketObject(),
		},
	}

	provider.ConfigureContextFunc = func(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
		var diags diag.Diagnostics
		config, err := providerConfigure(ctx, d)
		if err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Error configuring provider",
				Detail:   err.Error(),
			})
			return nil, diags
		}
		return config, diags
	}
	return provider
}

var descriptions map[string]string
var endpointServiceNames []string

func init() {
	descriptions = map[string]string{
		"region": "The region where Rabata Storage operations will take place. Examples\n" +
			"are us-east-1, eu-west-1, etc.",

		"access_key": "The access key for API operations. You can retrieve this\n" +
			"from the 'Security & Credentials' section of the Rabata console.",

		"secret_key": "The secret key for API operations. You can retrieve this\n" +
			"from the 'Security & Credentials' section of the Rabata console.",

		"profile": "The profile for API operations. If not set, the default profile\n" +
			"created with `aws configure` will be used.",

		"shared_credentials_file": "The path to the shared credentials file. If not set\n" +
			"this defaults to ~/.aws/credentials.",

		"token": "session token. A session token is only required if you are\n" +
			"using temporary security credentials.",

		"max_retries": "The maximum number of times an Rabata API request is\n" +
			"being executed. If the API request still fails, an error is\n" +
			"thrown.",

		"endpoint": "Use this to override the default service endpoint URL",

		"skip_credentials_validation": "Skip the credentials validation via STS API. " +
			"Used for AWS API implementations that do not have STS available/implemented.",

		"skip_requesting_account_id": "Skip requesting the account ID. " +
			"Used for AWS API implementations that do not have IAM/STS API and/or metadata API.",

		"skip_medatadata_api_check": "Skip the AWS Metadata API check. " +
			"Used for AWS API implementations that do not have a metadata api endpoint.",

		"s3_force_path_style": "Set this to true to force the request to use path-style addressing,\n" +
			"i.e., http://s3.amazonaws.com/BUCKET/KEY. By default, the S3 client will\n" +
			"use virtual hosted bucket addressing when possible\n" +
			"(http://BUCKET.s3.amazonaws.com/KEY). Specific to the Amazon S3 service.",
	}

	endpointServiceNames = []string{
		"s3",
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, error) {
	region := d.Get("region").(string)

	endpoint, err := rabata_endpoints.RabataEndpoint(region)
	if err != nil {
		return nil, err
	}

	config := Config{
		AccessKey:     d.Get("access_key").(string),
		SecretKey:     d.Get("secret_key").(string),
		Profile:       d.Get("profile").(string),
		Token:         d.Get("token").(string),
		Region:        region,
		CredsFilename: d.Get("shared_credentials_file").(string),
		Endpoints: map[string]string{
			"s3": "https://s3." + endpoint,
		},
		MaxRetries:              d.Get("max_retries").(int),
		SkipCredsValidation:     true, //d.Get("skip_credentials_validation").(bool),
		SkipRequestingAccountId: d.Get("skip_requesting_account_id").(bool),
		SkipMetadataApiCheck:    d.Get("skip_metadata_api_check").(bool),
		S3ForcePathStyle:        d.Get("s3_force_path_style").(bool),
	}

	endpointsSet := d.Get("endpoints").(*schema.Set)

	for _, endpointsSetI := range endpointsSet.List() {
		endpoints := endpointsSetI.(map[string]interface{})
		for _, endpointServiceName := range endpointServiceNames {
			config.Endpoints[endpointServiceName] = endpoints[endpointServiceName].(string)
		}
	}

	return config.Client()
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
