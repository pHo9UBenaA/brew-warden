package homebrew

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"path"
	"strconv"
	"strings"
)

// Bound the extraction before calling native unpacking. Supported bottles may
// install payloads and relative links only inside their own versioned keg.
// Homebrew's .bottle/etc and .bottle/var restoration requires a broader action
// model, so those shared-prefix effects remain unsupported.
func validateBottleArchive(data []byte, f formulaMetadata) error {
	compressed, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return errors.New("invalid bottle archive")
	}
	defer compressed.Close()
	archive := tar.NewReader(compressed)
	version := f.Version
	if f.Revision > 0 {
		version += "_" + strconv.Itoa(f.Revision)
	}
	prefix := f.Name + "/" + version
	entries := map[string]byte{}
	var total int64
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("incomplete bottle archive")
		}
		name := strings.TrimSuffix(header.Name, "/")
		_, duplicate := entries[name]
		if !safeRelative(name) || duplicate || len(entries) >= 100000 || name != prefix && !strings.HasPrefix(name, prefix+"/") {
			return errors.New("unsafe or ambiguous bottle path")
		}
		relative := strings.TrimPrefix(name, prefix+"/")
		if relative == ".bottle" || strings.HasPrefix(relative, ".bottle/") || relative == "INSTALL_RECEIPT.json" {
			return errors.New("unsupported bottle installation side effects")
		}
		if header.Mode&07000 != 0 || header.Mode&^07777 != 0 {
			return errors.New("unsupported bottle permissions")
		}
		entries[name] = header.Typeflag
		switch header.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg:
			if header.Size < 0 || header.Size > 128*1024*1024 {
				return errors.New("bottle file exceeds limit")
			}
			total += header.Size
			if total > 2*1024*1024*1024 {
				return errors.New("bottle payload exceeds limit")
			}
		case tar.TypeSymlink:
			target := path.Clean(path.Join(path.Dir(name), header.Linkname))
			if path.IsAbs(header.Linkname) || target != prefix && !strings.HasPrefix(target, prefix+"/") {
				return errors.New("bottle link escapes keg")
			}
		default:
			return errors.New("unsupported bottle archive entry")
		}
	}
	if entries[prefix] != tar.TypeDir || len(entries) < 2 {
		return errors.New("empty bottle payload")
	}
	for name := range entries {
		for parent := path.Dir(name); parent != f.Name && parent != "."; parent = path.Dir(parent) {
			if kind, ok := entries[parent]; ok && kind != tar.TypeDir {
				return errors.New("bottle path traverses non-directory")
			}
		}
	}
	// Consume bounded tar padding and validate the gzip footer, including CRC.
	n, err := io.Copy(io.Discard, io.LimitReader(compressed, 1024*1024+1))
	if err != nil || n > 1024*1024 {
		return errors.New("invalid bottle archive trailer")
	}
	return nil
}
