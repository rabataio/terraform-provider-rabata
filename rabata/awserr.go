package rabata

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

// Returns true if the error matches all these conditions:
//   - err is of type awserr.Error
//   - Error.Code() matches code
//   - Error.Message() contains message
func isAWSErr(err error, code string, message string) bool { //nolint:unparam
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == code && strings.Contains(awsErr.Message(), message)
	}

	return false
}

// Returns true if the error matches all these conditions:
//   - err is of type awserr.RequestFailure
//   - RequestFailure.StatusCode() matches status code
//
// It is always preferable to use isAWSErr() except in older APIs (e.g. S3)
// that sometimes only respond with status codes.
func isAWSErrRequestFailureStatusCode(err error, statusCode int) bool {
	var awsErr awserr.RequestFailure
	if errors.As(err, &awsErr) {
		return awsErr.StatusCode() == statusCode
	}

	return false
}

func retryOnAWSCode(ctx context.Context, code string, f func() (any, error)) (any, error) {
	var resp any

	err := retry.RetryContext(ctx, 2*time.Minute, func() *retry.RetryError { //nolint:mnd
		var err error

		resp, err = f()
		if err != nil {
			var awsErr awserr.Error

			ok := errors.As(err, &awsErr)
			if ok && awsErr.Code() == code {
				return retry.RetryableError(err)
			}

			return retry.NonRetryableError(err)
		}

		return nil
	})

	return resp, err
}
