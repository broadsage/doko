package builder

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha1" //nolint:gosec // APK format requires SHA-1 checksums per spec
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/broadsage/doko/internal/config"
)

type writerCounter struct {
	writer io.Writer
	count  atomic.Uint64
}

func (c *writerCounter) Write(p []byte) (n int, err error) {
	n, err = c.writer.Write(p)
	if err != nil {
		return n, fmt.Errorf("counter write: %w", err)
	}
	c.count.Add(uint64(n))
	return n, nil
}

func (c *writerCounter) countVal() uint64 { return c.count.Load() }

func writeTarDir(tw *tar.Writer, h *tar.Header) error {
	h.ChangeTime = time.Time{}
	h.AccessTime = time.Time{}
	h.Format = tar.FormatPAX
	if err := tw.WriteHeader(h); err != nil {
		return fmt.Errorf("write tar dir header: %w", err)
	}
	return nil
}

func writeTarFile(tw *tar.Writer, h *tar.Header, r io.Reader) error {
	h.Format = tar.FormatPAX
	h.ChangeTime = time.Time{}
	h.AccessTime = time.Time{}
	if err := tw.WriteHeader(h); err != nil {
		return fmt.Errorf("write tar file header: %w", err)
	}
	if _, err := io.Copy(tw, r); err != nil {
		return fmt.Errorf("write tar file content: %w", err)
	}
	return nil
}

type tarKind int

const (
	tarFull tarKind = iota
	tarCut
)

// writeTgz writes a tgz stream to w, optionally padding to 512-byte boundary for tarFull.
// digest receives the same bytes as w. Returns the digest sum.
func writeTgz(w io.Writer, kind tarKind, build func(tw *tar.Writer) error, digest hash.Hash) ([]byte, error) {
	mw := io.MultiWriter(digest, w)
	gz := gzip.NewWriter(mw)
	cw := &writerCounter{writer: gz}
	bw := bufio.NewWriterSize(cw, 4096)
	tw := tar.NewWriter(bw)

	if err := build(tw); err != nil {
		return nil, err
	}

	bw.Flush()
	tw.Close()
	if kind == tarFull {
		bw.Flush()
	}

	size := cw.countVal()
	aligned := (size + 511) & ^uint64(511)
	if pad := aligned - size; pad > 0 {
		if _, err := cw.Write(make([]byte, pad)); err != nil {
			return nil, err
		}
	}

	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("close gzip writer: %w", err)
	}
	return digest.Sum(nil), nil
}

func mapArch(arch string) string {
	switch strings.ToLower(arch) {
	case "amd64", "x86_64":
		return "x86_64"
	case "arm64", "aarch64":
		return "aarch64"
	case "386", "x86":
		return "x86"
	default:
		return "noarch"
	}
}

// AssembleAPK writes an APK file to outPath by packing the directory dataDir.
// Metadata comes from the spec; built files under dataDir are packed as the data tgz,
// then .PKGINFO and control tgz are built, and control + data are concatenated into the APK.
// No signing (signature tgz is omitted).
func AssembleAPK(dataDir, outPath string, s *config.BuildSpec, arch string) error {
	dataDir = filepath.Clean(dataDir)
	if dataDir == "" || dataDir == "." {
		dataDir, _ = filepath.Abs(dataDir)
	}

	var dataSize int64
	dataDigest := sha256.New()
	dataTgz, err := os.CreateTemp("", "apk-data-*.tgz")
	if err != nil {
		return fmt.Errorf("create data temp file: %w", err)
	}
	defer os.Remove(dataTgz.Name())
	defer dataTgz.Close()

	dataHash, err := writeTgz(dataTgz, tarFull, func(tw *tar.Writer) error {
		return filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(dataDir, path)
			if err != nil {
				return fmt.Errorf("compute relative path: %w", err)
			}
			// APK expects forward slashes and no leading ./
			name := filepath.ToSlash(rel)
			if name == "." {
				return nil
			}
			h, err := tar.FileInfoHeader(info, name)
			if err != nil {
				return fmt.Errorf("build tar header: %w", err)
			}
			h.Name = name

			if info.IsDir() {
				return writeTarDir(tw, h)
			}

			f, err := os.Open(path) //nolint:gosec // Walk callback path is trusted (local dataDir)
			if err != nil {
				return fmt.Errorf("open file %s: %w", path, err)
			}
			defer f.Close()

			h1 := sha1.New() //nolint:gosec // APK-TOOLS.checksum.SHA1 mandates SHA-1
			if _, err := io.Copy(h1, f); err != nil {
				return fmt.Errorf("compute sha1 checksum: %w", err)
			}
			sha1Hex := hex.EncodeToString(h1.Sum(nil))
			h.PAXRecords = map[string]string{
				"APK-TOOLS.checksum.SHA1": sha1Hex,
			}

			// Seek back to start to write content to tarball
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return fmt.Errorf("seek file: %w", err)
			}

			dataSize += info.Size()
			err = writeTarFile(tw, h, f)
			return err
		})
	}, dataDigest)
	if err != nil {
		return fmt.Errorf("data tgz: %w", err)
	}

	pkgrel := "0"
	if s.Epoch > 0 {
		pkgrel = fmt.Sprintf("%d", s.Epoch)
	}
	pkgver := fmt.Sprintf("%s-r%s", s.Version, pkgrel)
	// .PKGINFO format (key = value, one per line)
	var pkginfo strings.Builder
	pkginfo.WriteString("# Generated\n")
	fmt.Fprintf(&pkginfo, "pkgname = %s\n", strings.ToLower(s.Name))
	fmt.Fprintf(&pkginfo, "pkgver = %s\n", pkgver)
	fmt.Fprintf(&pkginfo, "pkgdesc = %s\n", s.Description)
	fmt.Fprintf(&pkginfo, "url = %s\n", s.URL)
	fmt.Fprintf(&pkginfo, "arch = %s\n", mapArch(arch))
	fmt.Fprintf(&pkginfo, "size = %d\n", dataSize)
	fmt.Fprintf(&pkginfo, "datahash = %s\n", hex.EncodeToString(dataHash))
	if s.License != "" {
		fmt.Fprintf(&pkginfo, "license = %s\n", s.License)
	}
	for _, d := range s.Dependencies {
		fmt.Fprintf(&pkginfo, "depend = %s\n", d)
	}
	pkginfoBytes := pkginfo.String()

	controlTgz, err := os.CreateTemp("", "apk-control-*.tgz")
	if err != nil {
		return fmt.Errorf("create control temp file: %w", err)
	}
	defer os.Remove(controlTgz.Name())
	defer controlTgz.Close()

	_, err = writeTgz(controlTgz, tarCut, func(tw *tar.Writer) error {
		h := &tar.Header{
			Name: ".PKGINFO",
			Mode: 0o600,
			Size: int64(len(pkginfoBytes)),
		}
		return writeTarFile(tw, h, strings.NewReader(pkginfoBytes))
	}, sha1.New()) //nolint:gosec // APK control archive uses SHA-1 digest
	if err != nil {
		return fmt.Errorf("control tgz: %w", err)
	}

	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output APK: %w", err)
	}
	defer out.Close()

	// APK = control + data (no signature)
	for _, f := range []*os.File{controlTgz, dataTgz} {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("seek for copy: %w", err)
		}
		if _, err := io.Copy(out, f); err != nil {
			return fmt.Errorf("write APK segment: %w", err)
		}
	}
	return nil
}
