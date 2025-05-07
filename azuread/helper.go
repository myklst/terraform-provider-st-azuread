package azuread

import (
	"errors"

	"github.com/cenkalti/backoff"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

func handleAPIError(err error) error {
	var graphErr *odataerrors.ODataError

	if errors.As(err, &graphErr) {
		if isAbleToRetry(graphErr.GetStatusCode()) {
			return err
		} else {
			return backoff.Permanent(err)
		}
	} else {
		return backoff.Permanent(err)
	}
}
