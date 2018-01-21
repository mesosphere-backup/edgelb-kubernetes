package util

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

// ModDebugResp CONSUMES the response
func ModDebugResp(r *http.Response) string {
	return fmt.Sprintf("%s, %s", DebugResp(r), ModDebugRespBody(r))
}

// DebugResp prints some contents of the response
func DebugResp(r *http.Response) string {
	msg := "[[STATUS]]: %s, [[HEADER]]: %s"
	return fmt.Sprintf(msg, r.Status, r.Header)
}

// ModDebugRespBody CONSUMES the body
func ModDebugRespBody(r *http.Response) string {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		msg := "failed to read [[BODY]]: %s"
		return fmt.Sprintf(msg, err)
	}
	return fmt.Sprintf("[[BODY]]: %s", string(body))
}

// MsgWithClose will close and append the error if there is one.
func MsgWithClose(s string, c io.Closer) string {
	if err := c.Close(); err != nil {
		return fmt.Sprintf("(%s :: close err: %s)", s, err.Error())
	}
	return s
}
