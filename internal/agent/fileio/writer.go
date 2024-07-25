package fileio

import (
	"io"
	"os"
	"path"
)

// Writer is a struct for writing files to the device
type Writer struct {
	// rootDir is the root directory for the device writer useful for testing
	rootDir string
}

// New creates a new writer
func NewWriter() *Writer {
	return &Writer{}
}

// SetRootdir sets the root directory for the writer, useful for testing
func (r *Writer) SetRootdir(path string) {
	r.rootDir = path
}

// PathFor returns the full path for the provided file, useful for using functions
// and libraries that don't work with the fileio.Writer
func (r *Writer) PathFor(filePath string) string {
	return path.Join(r.rootDir, filePath)
}

// WriteFile writes the file at the provided path
func (r *Writer) WriteFile(filePath string, data []byte) error {
	return os.WriteFile(r.PathFor(filePath), data, 0644)
}

// WriteStreamToFile write the stream to the file at the provided path
func (r *Writer) WriteStreamToFile(filePath string, stream io.ReadCloser) error {
	outFile, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer outFile.Close()
	_, err = io.Copy(outFile, stream)
	return err
}
