// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// goFilesPackage creates a package for building a collection of Go files.
//
// This function is based on the function of the same name in cmd/go.
func goFilesPackage(gofiles []string) (*build.Package, error) {
	for _, f := range gofiles {
		if !strings.HasSuffix(f, ".go") {
			return nil, fmt.Errorf("named files must be .go files")
		}
	}

	buildctx := *buildctx
	buildctx.UseAllFiles = true

	// Synthesize fake "directory" that only shows the named files,
	// to make it look like this is a standard package or
	// command directory.  So that local imports resolve
	// consistently, the files must all be in the same directory.
	var dirent []os.FileInfo
	var dir string
	for _, file := range gofiles {
		fi, err := os.Stat(file)
		if err != nil {
			return nil, err
		}
		if fi.IsDir() {
			return nil, fmt.Errorf("%s is a directory, should be a Go file", file)
		}
		dir1, _ := filepath.Split(file)
		if dir == "" {
			dir = dir1
		} else if dir != dir1 {
			fmt.Errorf("named files must all be in one directory; have %s and %s", dir, dir1)
		}
		dirent = append(dirent, fi)
	}
	buildctx.ReadDir = func(string) ([]os.FileInfo, error) { return dirent, nil }

	if !filepath.IsAbs(dir) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(cwd, dir)
	}
	return buildctx.ImportDir(dir, 0)
}

// moveFile moves src to dst, catering for filesystem differences.
func moveFile(src, dst string) error {
	if printcommands {
		if dst == "-" {
			log.Printf("cat %s\n", src)
		} else {
			log.Printf("mv %s %s\n", src, dst)
		}
	}
	if dst == "-" {
		fin, err := os.Open(src)
		if err != nil {
			return err
		}
		defer fin.Close()
		_, err = io.Copy(os.Stdout, fin)
		return err
	}
	if os.Rename(src, dst) != nil {
		// rename may fail if the paths are on
		// different filesystems.
		fin, err := os.Open(src)
		if err != nil {
			return err
		}
		defer fin.Close()

		fout, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer fout.Close()

		info, err := fin.Stat()
		if err != nil {
			return err
		}
		if err = fout.Chmod(info.Mode()); err != nil {
			return err
		}
		if _, err = io.Copy(fout, fin); err != nil {
			return err
		}
		return os.Remove(src)
	}
	return nil
}

var gccgoExternRegexp = regexp.MustCompile("(?m)^//extern ")

// translateGccgoExterns rewrites a specified file so that
// any lines beginning with "//extern " are rewritten to
// use llgo's equivalent "// #llgo name: ".
func translateGccgoExterns(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	data = gccgoExternRegexp.ReplaceAllLiteral(data, []byte("// #llgo name: "))
	return ioutil.WriteFile(filename, data, 0644)
}

// find the library directory for the version
// of gcc found in $PATH.
func findGcclib() (string, error) {
	// TODO(axw) allow the use of $CC in place of cc. If we do this,
	// we'll need to work around partial installations of gcc, e.g.
	// ones without g++ (thus lacking libstdc++).
	var buf bytes.Buffer
	cmd := exec.Command("gcc", "--print-libgcc-file-name")
	cmd.Stderr = os.Stderr
	cmd.Stdout = &buf
	if err := runCmd(cmd); err != nil {
		return "", err
	}
	libfile := buf.String()
	return filepath.Dir(libfile), nil
}

// envFields gets the environment variable with the
// specified name and splits it into fields separated
// by whitespace.
func envFields(key string) []string {
	return strings.Fields(os.Getenv(key))
}
