package httptimeout

import (
	"errors"
	"net/url"
)

// StripURL returns the cause of a *url.Error and err itself otherwise.
//
// Purpose: net/http quotes the whole request URL in a transport error, so a
// credential sent in a query string would reach logs and terminals.
// Inputs: an error from http.Client.Do. Outputs: the same error without the URL.
func StripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
