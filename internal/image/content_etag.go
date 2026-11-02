package image

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ContentETag returns a strong validator for the exact representation and
// restores the reader to its beginning for http.ServeContent. Hashing is
// streamed with bounded memory and stops when the request is cancelled.
func ContentETag(ctx context.Context, reader io.ReadSeeker) (string, error) {
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	buf := make([]byte, 1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := reader.Read(buf)
		if n > 0 {
			_, _ = hash.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if n == 0 {
			return "", io.ErrNoProgress
		}
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return fmt.Sprintf(`"%x"`, hash.Sum(nil)), nil
}

// ovaETag hashes the OVA's ordered TAR headers and small members. The second
// member is the ISO: its immutable generation digest and exact ignition archive
// identify its overlaid bytes without reading the multi-GB stream per request.
func ovaETag(entries []TarEntry, isoDigest string, ignitionArchive []byte) (string, error) {
	hash := sha256.New()
	// Frame every component so different boundaries cannot collide.
	writePart := func(data []byte) {
		_, _ = fmt.Fprintf(hash, "%d:", len(data))
		_, _ = hash.Write(data)
	}
	writePart([]byte("migration-planner-ova-v1"))
	for i, entry := range entries {
		header, err := buildTarHeaderBytes(entry.Name, entry.Size, entry.Mode, entry.ModTime)
		if err != nil {
			return "", err
		}
		writePart(header)
		if i == 1 {
			writePart([]byte(isoDigest))
			writePart(ignitionArchive)
			continue
		}
		content, err := io.ReadAll(entry.Reader)
		if err != nil {
			return "", err
		}
		if _, err := entry.Reader.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		writePart(content)
	}
	return fmt.Sprintf(`"%x"`, hash.Sum(nil)), nil
}

// ServeOVA only honors ranges tied to a representation validator. Otherwise a
// refresh or a different pod could return a slice of a different OVA under the
// same download URL. HTTP permits returning the complete representation instead.
func ServeOVA(writer http.ResponseWriter, request *http.Request, name, etag string, reader io.ReadSeeker) {
	writer.Header().Set("ETag", etag)
	if request.Header.Get("Range") != "" && request.Header.Get("If-Range") == "" && strings.TrimSpace(request.Header.Get("If-Match")) != etag {
		request = request.Clone(request.Context())
		request.Header.Del("Range")
	}
	// Source creation time does not describe the changing representation.
	// Suppress Last-Modified so resumptions must use the strong ETag.
	http.ServeContent(writer, request, name, time.Time{}, reader)
}
