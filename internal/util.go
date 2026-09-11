package abmon

// Common abmon functions
// Author: Anders Lowinger, anders@abundo.se

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
)

// print structures JSON formatted
func Pprint(data interface{}) {
	s, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(s))
}

// Return data as JSON formatted string
func StructToString(data interface{}) string {
	s, _ := json.MarshalIndent(data, "", "  ")
	return string(s)
}

// Decide if two files have the same contents or not.
// chunkSize is the size of the blocks to scan by
// *Follows* symlinks.
//
// May return an error if something else goes wrong;
//
//	in this case, you should ignore the value of 'same'.
//
// derived from https://stackoverflow.com/a/30038571
// under CC-BY-SA-4.0 by several contributors
func FileCmp(file1, file2 string) (same bool, err error) {
	chunkSize := 4096

	// shortcuts: check file metadata
	stat1, err := os.Stat(file1)
	if err != nil {
		return false, err
	}

	stat2, err := os.Stat(file2)
	if err != nil {
		return false, err
	}

	// are inputs are literally the same file?
	if os.SameFile(stat1, stat2) {
		return true, nil
	}

	// do inputs at least have the same size?
	if stat1.Size() != stat2.Size() {
		return false, nil
	}

	// long way: compare contents
	f1, err := os.Open(file1)
	if err != nil {
		return false, err
	}
	defer f1.Close()

	f2, err := os.Open(file2)
	if err != nil {
		return false, err
	}
	defer f2.Close()

	b1 := make([]byte, chunkSize)
	b2 := make([]byte, chunkSize)
	for {
		n1, err1 := io.ReadFull(f1, b1)
		n2, err2 := io.ReadFull(f2, b2)

		// https://pkg.go.dev/io#Reader
		// > Callers should always process the n > 0 bytes returned
		// > before considering the error err. Doing so correctly
		// > handles I/O errors that happen after reading some bytes
		// > and also both of the allowed EOF behaviors.

		if !bytes.Equal(b1[:n1], b2[:n2]) {
			return false, nil
		}

		if (err1 == io.EOF && err2 == io.EOF) || (err1 == io.ErrUnexpectedEOF && err2 == io.ErrUnexpectedEOF) {
			return true, nil
		}

		// some other error, like a dropped network connection or a bad transfer
		if err1 != nil {
			return false, err1
		}
		if err2 != nil {
			return false, err2
		}
	}
}

// Copy file
func CopyFile(src, dst string) error {
	// Open the source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer sourceFile.Close()

	// Create the destination file
	destinationFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer destinationFile.Close()

	// Copy the contents from the source file to the destination file
	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}

	// Flush the write buffers to the underlying storage
	err = destinationFile.Sync()
	if err != nil {
		return fmt.Errorf("failed to flush to disk: %v", err)
	}

	return nil
}

// Return a sorted array of the keys of a map
func SortedKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j])
	})
	return keys
}
