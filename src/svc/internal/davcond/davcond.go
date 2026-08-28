package davcond

import (
	"errors"
	"net/http"

	"github.com/emersion/go-webdav"
)

func Check(existingETag string, exists bool, ifMatch, ifNoneMatch webdav.ConditionalMatch) error {
	pre := webdav.NewHTTPError(http.StatusPreconditionFailed, errors.New("precondition failed"))
	if ifNoneMatch.IsSet() {
		if ifNoneMatch.IsWildcard() && exists {
			return pre
		}
		if exists && existingETag != "" {
			if ok, err := ifNoneMatch.MatchETag(existingETag); err == nil && ok {
				return pre
			}
		}
	}
	if ifMatch.IsSet() {
		if !exists {
			return pre
		}
		if ifMatch.IsWildcard() {
			return nil
		}
		ok, err := ifMatch.MatchETag(existingETag)
		if err != nil || !ok {
			return pre
		}
	}
	return nil
}
