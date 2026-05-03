package image

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"
)

// section represents a contiguous range of bytes in the virtual TAR file,
// backed by an io.ReadSeeker.
type section struct {
	offset int64         // absolute start offset within the full TAR
	length int64         // byte length of this section
	reader io.ReadSeeker // data source (positioned relative to section start)
}

// SeekableTarReader implements io.ReadSeeker over a virtual TAR file composed
// of multiple sections. This allows http.ServeContent to handle byte-range
// requests without materializing the entire TAR on disk.
type SeekableTarReader struct {
	sections []section
	total    int64
	pos      int64
	closer   io.Closer // typically the ISO overlay reader
}

// zeroReader provides an infinite stream of zero bytes, used for TAR padding
// and the end-of-archive marker. It implements io.ReadSeeker.
type zeroReader struct {
	size int64
	pos  int64
}

func newZeroReader(size int64) *zeroReader {
	return &zeroReader{size: size}
}

func (z *zeroReader) Read(p []byte) (int, error) {
	remaining := z.size - z.pos
	if remaining <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > remaining {
		n = remaining
	}
	clear(p[:n])
	z.pos += n
	return int(n), nil
}

func (z *zeroReader) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = z.pos + offset
	case io.SeekEnd:
		newPos = z.size + offset
	default:
		return 0, errors.New("seekable_tar: invalid whence")
	}
	if newPos < 0 {
		return 0, errors.New("seekable_tar: negative position")
	}
	z.pos = newPos
	return newPos, nil
}

// NewSeekableTarReader constructs a seekable TAR reader from the given file entries.
// Each entry is a TAR member with a header and content reader. The modTime is used
// for all TAR headers to ensure deterministic output across pods.
func NewSeekableTarReader(entries []TarEntry, closer io.Closer) (*SeekableTarReader, int64, error) {
	var sections []section
	var offset int64

	for _, entry := range entries {
		// Generate TAR header bytes
		headerBytes, err := buildTarHeaderBytes(entry.Name, entry.Size, entry.Mode, entry.ModTime)
		if err != nil {
			return nil, 0, err
		}

		// Section: TAR header
		sections = append(sections, section{
			offset: offset,
			length: int64(len(headerBytes)),
			reader: bytes.NewReader(headerBytes),
		})
		offset += int64(len(headerBytes))

		// Section: file content
		if entry.Size > 0 {
			sections = append(sections, section{
				offset: offset,
				length: entry.Size,
				reader: entry.Reader,
			})
			offset += entry.Size
		}

		// Section: padding to 512-byte boundary
		padding := (512 - (entry.Size % 512)) % 512
		if padding > 0 {
			sections = append(sections, section{
				offset: offset,
				length: padding,
				reader: newZeroReader(padding),
			})
			offset += padding
		}
	}

	// End-of-archive: two 512-byte zero blocks
	const endOfArchiveSize = 1024
	sections = append(sections, section{
		offset: offset,
		length: endOfArchiveSize,
		reader: newZeroReader(endOfArchiveSize),
	})
	offset += endOfArchiveSize

	return &SeekableTarReader{
		sections: sections,
		total:    offset,
		closer:   closer,
	}, offset, nil
}

// TarEntry describes a single file within the TAR archive.
type TarEntry struct {
	Name    string
	Size    int64
	Mode    int64
	ModTime time.Time
	Reader  io.ReadSeeker
}

func (r *SeekableTarReader) Read(p []byte) (int, error) {
	if r.pos >= r.total {
		return 0, io.EOF
	}

	totalRead := 0
	for totalRead < len(p) && r.pos < r.total {
		// Find the section containing the current position
		sec := r.findSection(r.pos)
		if sec == nil {
			return totalRead, io.EOF
		}

		// Seek the section's reader to the relative offset
		relOffset := r.pos - sec.offset
		if _, err := sec.reader.Seek(relOffset, io.SeekStart); err != nil {
			return totalRead, err
		}

		// Read up to the end of this section
		remaining := sec.length - relOffset
		toRead := int64(len(p) - totalRead)
		if toRead > remaining {
			toRead = remaining
		}

		n, err := sec.reader.Read(p[totalRead : totalRead+int(toRead)])
		totalRead += n
		r.pos += int64(n)

		if n == 0 && err == nil {
			return totalRead, io.ErrNoProgress
		}
		if err != nil && err != io.EOF {
			return totalRead, err
		}
	}

	if totalRead == 0 && r.pos >= r.total {
		return 0, io.EOF
	}
	return totalRead, nil
}

func (r *SeekableTarReader) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = r.pos + offset
	case io.SeekEnd:
		newPos = r.total + offset
	default:
		return 0, errors.New("seekable_tar: invalid whence")
	}
	if newPos < 0 {
		return 0, errors.New("seekable_tar: negative position")
	}
	r.pos = newPos
	return newPos, nil
}

func (r *SeekableTarReader) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

func (r *SeekableTarReader) findSection(pos int64) *section {
	for i := range r.sections {
		s := &r.sections[i]
		if pos >= s.offset && pos < s.offset+s.length {
			return s
		}
	}
	return nil
}

// buildTarHeaderBytes generates the raw bytes of a single 512-byte TAR header block.
// The name must be short enough to fit in a USTAR header (≤100 chars) to avoid
// PAX extended headers which would produce more than 512 bytes.
func buildTarHeaderBytes(name string, size int64, mode int64, modTime time.Time) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := tw.WriteHeader(&tar.Header{
		Name:    name,
		Size:    size,
		Mode:    mode,
		ModTime: modTime,
	})
	if err != nil {
		return nil, err
	}
	if buf.Len() != 512 {
		return nil, fmt.Errorf("unexpected TAR header size %d for %q (expected 512; name may be too long)", buf.Len(), name)
	}
	return buf.Bytes(), nil
}
