package imagor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/cshum/imagor/imagorpath"
	"github.com/stretchr/testify/assert"
)

func TestWrapError(t *testing.T) {
	var err error
	var e Error

	assert.Equal(t, WrapError(nil), ErrInternal)

	assert.Equal(t, ErrMethodNotAllowed, WrapError(ErrMethodNotAllowed))

	err = NewError("errorrrr", 167)
	assert.Equal(t, WrapError(errors.New(err.Error())), err)

	assert.Equal(t, ErrTimeout, WrapError(context.DeadlineExceeded))

	assert.Equal(t, true, ErrTimeout.Timeout())

	assert.Equal(t, ErrTimeout, WrapError(&url.Error{Err: context.DeadlineExceeded}))

	err = errors.New("asdfsdfsaf")
	e = WrapError(err)
	assert.Equal(t, 500, e.Code)
	assert.Contains(t, e.Error(), err.Error())

	e = NewErrorFromStatusCode(403)
	assert.Equal(t, 403, e.Code)
	assert.Contains(t, e.Error(), http.StatusText(403))

	err = &net.DNSError{IsTimeout: true}
	assert.Equal(t, ErrTimeout, WrapError(err))

	err = ErrForward{imagorpath.Params{Width: 167, Height: 169, Image: "foo"}}
	assert.Equal(t, "imagor: forward 167x169/foo", err.Error())
	assert.Equal(t, ErrUnsupportedFormat, WrapError(err))

}

func TestWrapErrorPermanentSourceFailures(t *testing.T) {
	// Origin serves a certificate that does not match the source hostname: a
	// legacy host such as mag.ixbt.com pointing to a foreign certificate.
	certErr := &url.Error{
		Op:  "Get",
		URL: "https://mag.ixbt.com/img/topIpad.png",
		Err: &tls.CertificateVerificationError{Err: x509.HostnameError{
			Certificate: &x509.Certificate{}, Host: "mag.ixbt.com",
		}},
	}
	e := WrapError(certErr)
	assert.Equal(t, http.StatusNotFound, e.Code)
	assert.Contains(t, e.Message, "certificate")

	// Source hostname does not resolve.
	e = WrapError(&url.Error{
		Op:  "Get",
		URL: "https://gone.invalid/x.png",
		Err: &net.DNSError{Err: "no such host", Name: "gone.invalid", IsNotFound: true},
	})
	assert.Equal(t, http.StatusNotFound, e.Code)

	// Unknown authority is permanent as well.
	e = WrapError(x509.UnknownAuthorityError{})
	assert.Equal(t, http.StatusNotFound, e.Code)

	// Other generic failures stay internal errors.
	assert.Equal(t, http.StatusInternalServerError, WrapError(errors.New("boom")).Code)
}
