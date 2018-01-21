package client

import (
	"net/http"
	"runtime/debug"
	"testing"
)

func TestHasContentTypes(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{},
	}
	if !hasContentTypes(resp, []string{"application/octet-stream"}) {
		t.Fatalf("Missing header\n%s", debug.Stack())
	}
	resp.Header.Set("Content-Type", "application/json")
	if hasContentTypes(resp, []string{"text/html"}) {
		t.Fatalf("Extra header\n%s", debug.Stack())
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	if !hasContentTypes(resp, []string{"text/html"}) {
		t.Fatalf("Missing header\n%s", debug.Stack())
	}
	if hasContentTypes(resp, []string{"application/json"}) {
		t.Fatalf("Extra header\n%s", debug.Stack())
	}
}

func TestDcosURLHTTPHTTPS(t *testing.T) {
	testEdgelbURL(t, "mycluster.com/", "http", "mycluster.com", "service/edgelb")
	testEdgelbURL(t, "http://mycluster.com/", "http", "mycluster.com", "service/edgelb")
	testEdgelbURL(t, "https://mycluster.com/", "https", "mycluster.com", "service/edgelb")
	testEdgelbURL(t, "//mycluster.com", "http", "mycluster.com", "service/edgelb")
}

func testEdgelbURL(t *testing.T, clusterURL, expectedScheme, expectedHost, expectedBasePath string) {
	scheme, host, basePath, err := makeEdgelbURL(clusterURL, "edgelb")
	if err != nil {
		t.Fatal(err)
	}
	assertStringsEqual(t, expectedScheme, scheme, "Scheme was not properly set.")
	assertStringsEqual(t, expectedHost, host, "Host was not properly set.")
	assertStringsEqual(t, expectedBasePath, basePath, "Base path was not properly set.")
}

func assertStringsEqual(t *testing.T, s1, s2, msg string) {
	if s1 != s2 {
		t.Fatalf("%s Expected (%s), got (%s)\n%s", msg, s1, s2, debug.Stack())
	}
}
