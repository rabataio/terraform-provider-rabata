package rabata

import (
	"errors"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

func isResourceTimeoutError(err error) bool {
	var timeoutErr *retry.TimeoutError

	ok := errors.As(err, &timeoutErr)

	return ok && timeoutErr.LastError == nil
}
