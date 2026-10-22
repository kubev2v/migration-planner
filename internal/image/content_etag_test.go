package image

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestContentETagAndResumedDownload(t *testing.T) {
	old := []byte("old appliance OVA")
	current := []byte("new appliance OVA")
	oldTag, err := ContentETag(context.Background(), bytes.NewReader(old))
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(current)
	tag, err := ContentETag(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if tag == oldTag {
		t.Fatal("changed image retained its validator")
	}
	duplicateTag, err := ContentETag(context.Background(), bytes.NewReader(current))
	if err != nil || duplicateTag != tag {
		t.Fatalf("identical bytes have different validators: %s, %v", duplicateTag, err)
	}
	position, _ := reader.Seek(0, io.SeekCurrent)
	if position != 0 {
		t.Fatal("reader was not reset after hashing")
	}
	for _, tc := range []struct {
		name, validator string
		status          int
		body            []byte
	}{
		{"unchanged", tag, http.StatusPartialContent, current[4:]},
		{"changed", oldTag, http.StatusOK, current},
		{"unvalidated", "", http.StatusOK, current},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/image.ova", nil)
			req.Header.Set("Range", "bytes=4-")
			req.Header.Set("If-Range", tc.validator)
			response := httptest.NewRecorder()
			response.Header().Set("ETag", tag)
			ServeOVA(response, req, "image.ova", tag, bytes.NewReader(current))
			if response.Code != tc.status || !bytes.Equal(response.Body.Bytes(), tc.body) {
				t.Fatalf("response = %d %q, want %d %q", response.Code, response.Body.Bytes(), tc.status, tc.body)
			}
			if response.Header().Get("Last-Modified") != "" {
				t.Fatal("unsafe date validator advertised")
			}
		})
	}
}

// An ISO read is a failure here: computing validators, answering HEAD, and
// serving a range in the OVF must not scan the large ISO member.
type unreadableISO struct {
	*bytes.Reader
	t *testing.T
}

func (r unreadableISO) Read([]byte) (int, error) {
	r.t.Fatal("unexpected ISO read")
	return 0, io.ErrUnexpectedEOF
}

func TestOVAETagInputsAndCheapMetadataRequests(t *testing.T) {
	newEntries := func() []TarEntry {
		return []TarEntry{
			{Name: "assessment.ovf", Size: 3, Mode: 0600, Reader: bytes.NewReader([]byte("ovf"))},
			{Name: "assessment.iso", Size: 4 << 30, Mode: 0600, Reader: unreadableISO{bytes.NewReader(nil), t}},
			{Name: "disk.vmdk", Size: 4, Mode: 0600, Reader: bytes.NewReader([]byte("disk"))},
		}
	}
	entries := newEntries()
	tag, err := ovaETag(entries, "iso digest", []byte("ignition archive"))
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := ovaETag(newEntries(), "iso digest", []byte("ignition archive"))
	if err != nil || tag != duplicate {
		t.Fatalf("identical inputs changed ETag: %q, %v", duplicate, err)
	}
	for _, tc := range []struct {
		name     string
		change   func([]TarEntry)
		digest   string
		ignition string
	}{
		{name: "ISO", digest: "different ISO", ignition: "ignition archive"},
		{name: "ignition", digest: "iso digest", ignition: "different ignition"},
		{name: "OVF", change: func(e []TarEntry) { e[0].Reader = bytes.NewReader([]byte("new")) }},
		{name: "disk", change: func(e []TarEntry) { e[2].Reader = bytes.NewReader([]byte("next")) }},
		{name: "modification time", change: func(e []TarEntry) { e[0].ModTime = time.Unix(100, 0) }},
		{name: "member name", change: func(e []TarEntry) { e[0].Name = "other.ovf" }},
		{name: "member size", change: func(e []TarEntry) { e[1].Size++ }},
		{name: "member mode", change: func(e []TarEntry) { e[0].Mode = 0644 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEntries()
			if tc.change != nil {
				tc.change(e)
				tc.digest = "iso digest"
				tc.ignition = "ignition archive"
			}
			changed, err := ovaETag(e, tc.digest, []byte(tc.ignition))
			if err != nil || changed == tag {
				t.Fatalf("changed %s retained ETag: %q, %v", tc.name, changed, err)
			}
		})
	}
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		t.Run(method, func(t *testing.T) {
			reader, size, err := NewSeekableTarReader(entries, nil)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(method, "/image.ova", nil)
			if method == http.MethodGet {
				req.Header.Set("Range", "bytes=512-514")
				req.Header.Set("If-Range", tag)
			}
			response := httptest.NewRecorder()
			ServeOVA(response, req, "image.ova", tag, reader)
			if response.Header().Get("ETag") != tag {
				t.Fatal("HEAD and GET validators differ")
			}
			if method == http.MethodHead {
				if response.Code != http.StatusOK || response.Body.Len() != 0 || response.Result().ContentLength != size {
					t.Fatalf("invalid HEAD: %d, %v", response.Code, response.Header())
				}
			} else if response.Code != http.StatusPartialContent || response.Body.String() != "ovf" {
				t.Fatalf("invalid range: %d %q", response.Code, response.Body.String())
			}
		})
	}
}

func TestContentETagCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ContentETag(ctx, bytes.NewReader([]byte("OVA"))); err != context.Canceled {
		t.Fatalf("error = %v, want cancellation", err)
	}
}

func TestServeOVAHeadAndWildcard(t *testing.T) {
	data := []byte("appliance OVA")
	tag, err := ContentETag(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	head := httptest.NewRequest(http.MethodHead, "/image.ova", nil)
	response := httptest.NewRecorder()
	ServeOVA(response, head, "image.ova", tag, bytes.NewReader(data))
	if response.Code != http.StatusOK || response.Body.Len() != 0 || response.Header().Get("ETag") != tag || response.Header().Get("Content-Length") != "13" {
		t.Fatalf("invalid HEAD response: %d, %v, %q", response.Code, response.Header(), response.Body.String())
	}
	for _, validator := range []string{"*", " * ", `"different", *`} {
		request := httptest.NewRequest(http.MethodGet, "/image.ova", nil)
		request.Header.Set("Range", "bytes=4-")
		request.Header.Set("If-Match", validator)
		response = httptest.NewRecorder()
		ServeOVA(response, request, "image.ova", tag, bytes.NewReader(data))
		if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), data) {
			t.Fatalf("wildcard %q incorrectly permitted an unvalidated range: %d", validator, response.Code)
		}
	}
}
