package hydra

import (
	"archive/tar"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ghetzel/go-stockutil/convutil"
	"github.com/ghetzel/go-stockutil/fileutil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/sliceutil"
	yaml "gopkg.in/yaml.v2"
)

type ManifestFile struct {
	Name         string `yaml:"name"`
	Size         int64  `yaml:"size"`
	SHA256       string `yaml:"sha256"`
	MIME         string `yaml:"mime"`
	Archive      bool   `yaml:"archive,omitempty"`
	skipValidate bool
}

func (self *ManifestFile) stat(root string) (os.FileInfo, error) {
	return os.Stat(filepath.Join(root, self.Name))
}

func (self *ManifestFile) open(root string) (*os.File, error) {
	return os.Open(filepath.Join(root, self.Name))
}

// func (self *ManifestFile) ProperName() string {
// 	return strings.TrimSuffix(filepath.Base(self.Name), filepath.Ext(self.Name))
// }

func (self *ManifestFile) validate(root string) error {
	path := filepath.Join(root, self.Name)

	if fileutil.FileExists(path) {
		if cksum, err := fileutil.ChecksumFile(path, `sha256`); err == nil {
			if hex.EncodeToString(cksum) == self.SHA256 {
				return nil
			} else {
				return fmt.Errorf("invalid local file: ")
			}
		} else {
			return fmt.Errorf("malformed checksum")
		}
	} else {
		return fmt.Errorf("no such file")
	}
}

func (self *ManifestFile) fetch(root string) (io.ReadCloser, error) {
	return fetch(joinpath(root, self.Name))
}

type ManifestFiles []*ManifestFile

func (self ManifestFiles) TotalSize() (s convutil.Bytes) {
	for _, file := range self {
		s += convutil.Bytes(file.Size)
	}

	return
}

type Manifest struct {
	Assets        ManifestFiles `yaml:"assets"`
	Modules       ManifestFiles `yaml:"modules"`
	GlobalImports []string      `yaml:"globals,omitempty"`
	GeneratedAt   time.Time     `yaml:"generated_at,omitempty"`
	TotalSize     int64         `yaml:"size"`
	FileCount     int64         `yaml:"file_count"`
	rootDir       string
}

func (self *Manifest) LoadModules(fromDir string) (modules []*Module, err error) {
	for _, file := range self.Modules {
		module := new(Module)

		err = LoadModule(filepath.Join(fromDir, file.Name), module)

		if err == nil {
			module.Source = file.Name
			modules = append(modules, module)
		} else {
			return
		}
	}

	return
}

func (self *Manifest) Clean(destdir string) error {
	for _, module := range self.Modules {
		os.Remove(filepath.Join(destdir, module.Name))
	}

	return nil
}

func (self *Manifest) AddGlobalImportPath(path string) {
	if sliceutil.ContainsString(self.GlobalImports, path) {
		return
	}

	self.GlobalImports = append(self.GlobalImports, path)
}

func (self *Manifest) ShouldAppend(path string) bool {
	rel := self.rel(path)
	base := filepath.Base(rel)

	if rel == ManifestFilename {
		return false
	} else {
		switch base {
		case `qmldir`, ModuleSpecFilename, `Hydra.qml`:
			return false
		}
	}

	return true
}

func (self *Manifest) rel(path string) string {
	if self.rootDir != `` {
		if rel, err := filepath.Rel(self.rootDir, path); err == nil {
			return rel
		}
	}

	return path
}

func (self *Manifest) Append(path string, fi ...os.FileInfo) error {
	var info os.FileInfo

	if len(fi) > 0 && fi[0] != nil {
		info = fi[0]
	} else if fi, err := os.Stat(path); err == nil {
		info = fi
	} else {
		return err
	}

	if !info.IsDir() {
		if cksum, err := fileutil.ChecksumFile(path, `sha256`); err == nil {
			relPath := self.rel(path)

			switch base := filepath.Base(path); base {
			case ModuleSpecFilename:
				if spec, err := LoadModuleSpec(path); err == nil {
					if spec.Global {
						self.AddGlobalImportPath(filepath.Dir(path))
					}
				} else {
					return fmt.Errorf("create-manifest: invalid module spec %s: %v", path, err)
				}
			default:
				if !self.ShouldAppend(path) {
					return nil
				}
			}

			entry := &ManifestFile{
				Name:    relPath,
				Size:    info.Size(),
				SHA256:  hex.EncodeToString(cksum),
				MIME:    fileutil.GetMimeType(path),
				Archive: (getArchiveType(path) != NoArchive),
			}

			if IsValidModuleFile(path) {
				self.Modules = append(self.Modules, entry)
				log.Debugf("create-manifest: add module: %s (%v)", entry.Name, convutil.Bytes(entry.Size))
			} else {
				self.Assets = append(self.Assets, entry)
				log.Debugf("create-manifest:  add asset: %s (%v)", entry.Name, convutil.Bytes(entry.Size))
			}

			self.FileCount += 1
			self.TotalSize += info.Size()
		} else {
			return fmt.Errorf("create-manifest: %s: %v", path, err)
		}
	}

	return nil
}

func (self *Manifest) Fetch(srcroot string, destdir string) error {
	var toFetch ManifestFiles

	for _, file := range append(self.Assets, self.Modules...) {
		if err := file.validate(destdir); err != nil {
			toFetch = append(toFetch, file)
		}
	}

	if len(toFetch) > 0 {
		log.Infof("fetching %d files (%v) into %s", len(toFetch), toFetch.TotalSize(), destdir)

		for _, file := range toFetch {
			dest := filepath.Join(destdir, file.Name)
			log.Debugf("fetching file: %s[%s]", srcroot, dest)

			if rc, err := file.fetch(srcroot); err == nil {
				defer rc.Close()

				if _, err := fileutil.WriteFile(rc, dest); err == nil {
					rc.Close()
				} else {
					return fmt.Errorf("%s: write: %v", file.Name, err)
				}
			} else {
				return fmt.Errorf("%s: retrieve: %v", file.Name, err)
			}

			if file.Archive {
				if err := extract(self, dest, destdir); err == nil {
					file.skipValidate = true
				} else {
					return fmt.Errorf("%s: extract: %v", file.Name, err)
				}
			}
		}
	}

	for _, file := range self.Files() {
		if file.skipValidate {
			continue
		}

		if err := file.validate(destdir); err != nil {
			os.Remove(filepath.Join(destdir, file.Name))
			return fmt.Errorf("%s: invalid file: %v", filepath.Join(destdir, file.Name), err)
		}
	}

	return nil
}

func (self *Manifest) Files() ManifestFiles {
	return append(self.Assets, self.Modules...)
}

func (self *Manifest) isAutogenerated(file *ManifestFile) bool {
	if filepath.Ext(file.Name) == `.qml` {
		yamlFile := fileutil.SetExt(file.Name, `.yaml`)

		for _, mod := range self.Modules {
			if mod.Name == yamlFile {
				return true
			}
		}
	}

	return false
}

func (self *Manifest) Bundle(outfile string) error {
	if targz, err := os.Create(outfile); err == nil {
		gzw := gzip.NewWriter(targz)
		tw := tar.NewWriter(gzw)

		defer targz.Close()
		defer gzw.Close()
		defer tw.Close()

		for _, file := range self.Files() {
			if file.Archive {
				continue
			} else if self.isAutogenerated(file) {
				continue
			} else if err := file.validate(self.rootDir); err != nil {
				return fmt.Errorf("bundle: invalid file %s: %v", file.Name, err)
			}

			if stat, err := file.stat(self.rootDir); err == nil {
				if header, err := tar.FileInfoHeader(stat, stat.Name()); err == nil {
					log.Infof("bundling file: %s", file.Name)
					header.Name = file.Name

					// write the header
					if err := tw.WriteHeader(header); err != nil {
						return fmt.Errorf("bundle: header %s: %v", file.Name, err)
					}

					// copy file contents
					if f, err := file.open(self.rootDir); err == nil {
						defer f.Close()

						if _, err := io.Copy(tw, f); err == nil {
							f.Close()
						} else {
							return fmt.Errorf("bundle: archive %s: %v", file.Name, err)
						}
					} else {
						return fmt.Errorf("bundle: read %s: %v", file.Name, err)
					}
				} else {
					return fmt.Errorf("bundle: header %s: %v", file.Name, err)
				}
			} else {
				return fmt.Errorf("bundle: file %s: %v", file.Name, err)
			}
		}

		tw.Close()
		gzw.Close()
		targz.Close()
		log.Noticef("wrote bundle: %s (%v)", outfile, fileutil.SizeOf(outfile))
		return nil
	} else {
		return err
	}
}

func (self *Manifest) WriteFile(manifestFile string) error {
	var w io.Writer

	if manifestFile == `` {
		manifestFile = ManifestFilename
	}

	switch manifestFile {
	case `-`:
		w = os.Stdout
	default:
		if file, err := os.Create(manifestFile); err == nil {
			defer file.Close()
			w = file
		} else {
			log.Fatal(err)
		}
	}

	return yaml.NewEncoder(w).Encode(&Application{
		Manifest: self,
	})
}

// Wherever an app is being developed will have it live in a source tree.  This function will
// walk that tree and generate a manifest.yaml from it.
//
// Local paths will go into the manifest as relative to the approot.
// Remote paths will go into the manifest verbatim, UNLESS a flag is set to download them
// now (VENDORING), in which case they will be local paths.
// ----------------------------------------------------------------------------------------------
func CreateManifest(srcdir string) (*Manifest, error) {
	manifest := new(Manifest)
	manifest.rootDir = srcdir
	log.Infof("Generating manifest recursively from path: %s", srcdir)

	if err := filepath.Walk(srcdir, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			return manifest.Append(path, info)
		} else {
			return err
		}
	}); err == nil {
		manifest.GeneratedAt = time.Now()
		sort.Strings(manifest.GlobalImports)

		for i, gi := range manifest.GlobalImports {
			gi, _ = filepath.Rel(srcdir, gi)
			log.Debugf("create-manifest: global import: %s", gi)
			manifest.GlobalImports[i] = gi
		}

		return manifest, nil
	} else {
		return nil, err
	}
}
