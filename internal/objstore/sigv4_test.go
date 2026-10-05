package objstore

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

var vectorTime = time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)

func sigOf(t *testing.T, req *http.Request) string {
	t.Helper()
	a := req.Header.Get("Authorization")
	i := strings.Index(a, "Signature=")
	if i < 0 {
		t.Fatalf("no signature in %q", a)
	}
	return a[i+len("Signature="):]
}

func newReq(t *testing.T, method, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

// TestSigV4AWSVectors: known answers from the AWS SigV4 test suite (service
// "service", AKIDEXAMPLE, 20150830T123600Z) and from the Amazon S3 SigV4
// documentation examples (examplebucket, 20130524T000000Z).
func TestSigV4AWSVectors(t *testing.T) {
	suite := Signer{AccessKey: "AKIDEXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", Region: "us-east-1", Service: "service"}
	for _, c := range []struct{ name, method, url, want string }{
		{"get-vanilla", "GET", "http://example.amazonaws.com/", "5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"},
		{"get-vanilla-query-order-key-case", "GET", "http://example.amazonaws.com/?Param2=value2&Param1=value1", "b97d918cfa904a5beff61c982a1b6f458b799221646efd99d3219ec94cdf2500"},
		{"post-vanilla", "POST", "http://example.amazonaws.com/", "5da7c1a2acd57cee7505fc6676e4e544621c30862966e37dddb68e92efbe5d6b"},
	} {
		req := newReq(t, c.method, c.url)
		suite.Sign(req, emptySHA256, vectorTime)
		if got := sigOf(t, req); got != c.want {
			t.Errorf("%s: signature %s, want %s", c.name, got, c.want)
		}
		if !strings.Contains(req.Header.Get("Authorization"), "SignedHeaders=host;x-amz-date, ") {
			t.Errorf("%s: unexpected signed headers in %q", c.name, req.Header.Get("Authorization"))
		}
	}

	s3 := Signer{AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", Region: "us-east-1", Service: "s3"}
	at := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

	get := newReq(t, "GET", "https://examplebucket.s3.amazonaws.com/test.txt")
	get.Header.Set("Range", "bytes=0-9")
	s3.Sign(get, emptySHA256, at)
	if got, want := sigOf(t, get), "f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"; got != want {
		t.Errorf("s3 GET object with Range: %s, want %s", got, want)
	}

	list := newReq(t, "GET", "https://examplebucket.s3.amazonaws.com/?max-keys=2&prefix=J")
	s3.Sign(list, emptySHA256, at)
	if got, want := sigOf(t, list), "34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"; got != want {
		t.Errorf("s3 GET bucket list: %s, want %s", got, want)
	}

	put := newReq(t, "PUT", "https://examplebucket.s3.amazonaws.com/test%24file.text")
	put.Header.Set("Date", "Fri, 24 May 2013 00:00:00 GMT")
	put.Header.Set("X-Amz-Storage-Class", "REDUCED_REDUNDANCY")
	s3.Sign(put, "44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072", at)
	if got, want := sigOf(t, put), "98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd"; got != want {
		t.Errorf("s3 PUT object: %s, want %s", got, want)
	}
}

func TestURIEncode(t *testing.T) {
	cases := []struct {
		in    string
		slash bool
		want  string
	}{
		{"a b", false, "a%20b"},
		{"a+b", false, "a%2Bb"},
		{"100%", false, "100%25"},
		{"ключ", false, "%D0%BA%D0%BB%D1%8E%D1%87"},
		{"a/b", false, "a/b"},
		{"a/b", true, "a%2Fb"},
		{"A-z_0.9~", true, "A-z_0.9~"},
		{"=&?#", true, "%3D%26%3F%23"},
	}
	for _, c := range cases {
		if got := uriEncode(c.in, c.slash); got != c.want {
			t.Errorf("uriEncode(%q,%v) = %q, want %q", c.in, c.slash, got, c.want)
		}
	}
	// Query values: sorted, re-encoded, '+' kept distinct from space.
	if got, want := canonicalQuery("b=2&a=x+y&a=%2B&c="), "a=%2B&a=x%20y&b=2&c="; got != want {
		t.Errorf("canonicalQuery = %q, want %q", got, want)
	}
	if _, err := url.ParseQuery("continuation-token=" + uriEncode("a+b/c=d%e", true)); err != nil {
		t.Error(err)
	}
}
