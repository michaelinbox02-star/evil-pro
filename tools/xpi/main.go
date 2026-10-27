// Command xpi packs a directory into a Firefox .xpi.
//
// It exists instead of PowerShell's Compress-Archive / ZipFile for two reasons:
//   - .NET's zip helpers write backslash separators on Windows, which is not what
//     the zip spec says and which Firefox is picky about
//   - they fail outright if any file in the tree is held open for writing (a
//     freshly built host binary often is); Go opens with full share mode
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: xpi <srcdir> <out.xpi>")
		os.Exit(2)
	}
	src, out := os.Args[1], os.Args[2]

	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "not a directory: %s\n", src)
		os.Exit(1)
	}

	var files []string
	err = filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sort.Strings(files)

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	f, err := os.Create(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for _, p := range files {
		rel, err := filepath.Rel(src, p)
		if err != nil {
			continue
		}
		name := strings.ReplaceAll(rel, string(filepath.Separator), "/")
		w, err := zw.Create(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		in, err := os.Open(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			os.Exit(1)
		}
		if _, err := io.Copy(w, in); err != nil {
			in.Close()
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		in.Close()
	}
	if err := zw.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("packed %d files -> %s\n", len(files), out)
}
