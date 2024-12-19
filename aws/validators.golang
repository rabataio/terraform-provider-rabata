package aws

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

const (
	awsAccountIDRegexpPattern = `^(aws|\d{12})$`
	awsPartitionRegexpPattern = `^aws(-[a-z]+)*$`
	awsRegionRegexpPattern    = `^[a-z]{2}(-[a-z]+)+-\d$`
)

var awsAccountIDRegexp = regexp.MustCompile(awsAccountIDRegexpPattern)
var awsPartitionRegexp = regexp.MustCompile(awsPartitionRegexpPattern)
var awsRegionRegexp = regexp.MustCompile(awsRegionRegexpPattern)

func validateAccountAlias(v interface{}, k string) (ws []string, es []error) {
	val := v.(string)

	if (len(val) < 3) || (len(val) > 63) {
		es = append(es, fmt.Errorf("%q must contain from 3 to 63 alphanumeric characters or hyphens", k))
	}
	if !regexp.MustCompile("^[a-z0-9][a-z0-9-]+$").MatchString(val) {
		es = append(es, fmt.Errorf("%q must start with an alphanumeric character and only contain lowercase alphanumeric characters and hyphens", k))
	}
	if strings.Contains(val, "--") {
		es = append(es, fmt.Errorf("%q must not contain consecutive hyphens", k))
	}
	if strings.HasSuffix(val, "-") {
		es = append(es, fmt.Errorf("%q must not end in a hyphen", k))
	}
	return
}

func validateIAMPolicyJson(v interface{}, k string) (ws []string, errors []error) {
	// IAM Policy documents need to be valid JSON, and pass legacy parsing
	value := v.(string)
	if len(value) < 1 {
		errors = append(errors, fmt.Errorf("%q contains an invalid JSON policy", k))
		return
	}
	if value[:1] != "{" {
		errors = append(errors, fmt.Errorf("%q contains an invalid JSON policy", k))
		return
	}
	if _, err := structure.NormalizeJsonString(v); err != nil {
		errors = append(errors, fmt.Errorf("%q contains an invalid JSON: %s", k, err))
	}
	return
}

func validateOpenIdURL(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	u, err := url.Parse(value)
	if err != nil {
		errors = append(errors, fmt.Errorf("%q has to be a valid URL", k))
		return
	}
	if u.Scheme != "https" {
		errors = append(errors, fmt.Errorf("%q has to use HTTPS scheme (i.e. begin with https://)", k))
	}
	if len(u.Query()) > 0 {
		errors = append(errors, fmt.Errorf("%q cannot contain query parameters per the OIDC standard", k))
	}
	return
}

func validateIamRolePolicyName(v interface{}, k string) (ws []string, errors []error) {
	// https://github.com/boto/botocore/blob/2485f5c/botocore/data/iam/2010-05-08/service-2.json#L8291-L8296
	value := v.(string)
	if len(value) > 128 {
		errors = append(errors, fmt.Errorf(
			"%q cannot be longer than 128 characters", k))
	}
	if !regexp.MustCompile(`^[\w+=,.@-]+$`).MatchString(value) {
		errors = append(errors, fmt.Errorf(`%q must match [\w+=,.@-]`, k))
	}
	return
}

func validateIamRolePolicyNamePrefix(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	if len(value) > 100 {
		errors = append(errors, fmt.Errorf(
			"%q cannot be longer than 100 characters", k))
	}
	if !regexp.MustCompile(`^[\w+=,.@-]+$`).MatchString(value) {
		errors = append(errors, fmt.Errorf(`%q must match [\w+=,.@-]`, k))
	}
	return
}

func validateAwsAccountId(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)

	// http://docs.aws.amazon.com/lambda/latest/dg/API_AddPermission.html
	pattern := `^\d{12}$`
	if !regexp.MustCompile(pattern).MatchString(value) {
		errors = append(errors, fmt.Errorf(
			"%q doesn't look like AWS Account ID (exactly 12 digits): %q",
			k, value))
	}

	return
}

func validateArn(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)

	if value == "" {
		return
	}

	parsedARN, err := arn.Parse(value)

	if err != nil {
		errors = append(errors, fmt.Errorf("%q (%s) is an invalid ARN: %s", k, value, err))
		return
	}

	if parsedARN.Partition == "" {
		errors = append(errors, fmt.Errorf("%q (%s) is an invalid ARN: missing partition value", k, value))
	} else if !awsPartitionRegexp.MatchString(parsedARN.Partition) {
		errors = append(errors, fmt.Errorf("%q (%s) is an invalid ARN: invalid partition value (expecting to match regular expression: %s)", k, value, awsPartitionRegexpPattern))
	}

	if parsedARN.Region != "" && !awsRegionRegexp.MatchString(parsedARN.Region) {
		errors = append(errors, fmt.Errorf("%q (%s) is an invalid ARN: invalid region value (expecting to match regular expression: %s)", k, value, awsRegionRegexpPattern))
	}

	if parsedARN.AccountID != "" && !awsAccountIDRegexp.MatchString(parsedARN.AccountID) {
		errors = append(errors, fmt.Errorf("%q (%s) is an invalid ARN: invalid account ID value (expecting to match regular expression: %s)", k, value, awsAccountIDRegexpPattern))
	}

	if parsedARN.Resource == "" {
		errors = append(errors, fmt.Errorf("%q (%s) is an invalid ARN: missing resource value", k, value))
	}

	return
}

func validateS3BucketLifecycleTimestamp(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	_, err := time.Parse(time.RFC3339, fmt.Sprintf("%sT00:00:00Z", value))
	if err != nil {
		errors = append(errors, fmt.Errorf(
			"%q cannot be parsed as RFC3339 Timestamp Format", value))
	}

	return
}

func validateS3BucketLifecycleTransitionStorageClass() schema.SchemaValidateFunc {
	return validation.StringInSlice([]string{
		s3.TransitionStorageClassGlacier,
		s3.TransitionStorageClassStandardIa,
		s3.TransitionStorageClassOnezoneIa,
		s3.TransitionStorageClassIntelligentTiering,
		s3.TransitionStorageClassDeepArchive,
	}, false)
}
