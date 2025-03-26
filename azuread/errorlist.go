package azuread

const (
	ERR_TOO_MANY_REQ            = 429
	ERR_INTERNAL_ERROR          = 500
	ERR_SERVICE_UNAVAILABLE     = 503
	ERR_BANDWITH_LIMIT_EXCEEDED = 509
)

func isAbleToRetry(errCode int) bool {
	switch errCode {
	case
		ERR_TOO_MANY_REQ,
		ERR_INTERNAL_ERROR,
		ERR_SERVICE_UNAVAILABLE,
		ERR_BANDWITH_LIMIT_EXCEEDED:
		return true
	default:
		return false
	}
	// return false
}
