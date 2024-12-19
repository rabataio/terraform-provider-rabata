package aws

import (
	"errors"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func isResourceTimeoutError(err error) bool {
	var timeoutErr *resource.TimeoutError
	ok := errors.As(err, &timeoutErr)
	return ok && timeoutErr.LastError == nil
}
