package content

import (
	"crypto/sha1"
	"encoding/hex"
	"path"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// SidecarDir is where the CMS keeps block identities, mirroring the
// content tree: `src/content/pages/about.md` has its sidecar at
// `.norcube/cms/meta/src/content/pages/about.yml`.
const SidecarDir = ".norcube/cms/meta"

// Sidecar is one identity file.
type Sidecar struct {
	SchemaVersion int                   `yaml:"schemaVersion"`
	Page          string                `yaml:"page,omitempty"`
	Arrays        map[string][]ItemMeta `yaml:"arrays,omitempty"`
	// Translation, on a language version, records that a person (or
	// Norcube AI, confirmed by one) checked it against its source and
	// found it current.
	Translation *TranslationMark `yaml:"translation,omitempty"`
}

// TranslationMark says that a language version was current with its
// source as of one version of each: the source's file was SourceBlob and
// this version's own file Blob. While this version's file is still Blob,
// quality.translation_outdated compares the source with SourceBlob
// instead of with the source as it was when this version last changed.
// Once this version's file changes, the mark no longer applies: its own
// change is the newer base.
type TranslationMark struct {
	// Source is the source version's path, from the repository root.
	Source string `yaml:"source"`
	// SourceBlob and Blob are git blob ids (BlobID).
	SourceBlob string `yaml:"sourceBlob"`
	Blob       string `yaml:"blob"`
}

// Applies reports whether the mark speaks for a version whose file is
// blob, against the source at source: both named and still the same.
func (m *TranslationMark) Applies(source, blob string) bool {
	return m != nil && m.Source != "" && m.Source == source && m.SourceBlob != "" && m.Blob != "" && m.Blob == blob
}

// BlobID is the git object id of a file with these bytes (SHA-1, as
// `git hash-object` gives it).
func BlobID(raw []byte) string {
	h := sha1.New()
	h.Write([]byte("blob " + strconv.Itoa(len(raw)) + "\x00"))
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}

// ItemMeta is the identity of one array item.
type ItemMeta struct {
	ID     string                `yaml:"id"`
	Hashes []string              `yaml:"hashes,omitempty"`
	Arrays map[string][]ItemMeta `yaml:"arrays,omitempty"`
}

// SidecarPath returns the sidecar path of a content file.
func SidecarPath(contentPath string) string {
	cleaned := strings.Trim(strings.TrimSpace(contentPath), "/")
	if cleaned == "" {
		return ""
	}
	ext := path.Ext(cleaned)
	return path.Join(SidecarDir, strings.TrimSuffix(cleaned, ext)+".yml")
}

// ContentPathsForSidecar returns the content files a sidecar could
// belong to, one per content extension, since the sidecar drops the
// extension.
func ContentPathsForSidecar(sidecarPath string) []string {
	rel := strings.TrimPrefix(strings.Trim(sidecarPath, "/"), SidecarDir+"/")
	if rel == sidecarPath || rel == "" {
		return nil
	}
	base := strings.TrimSuffix(rel, ".yml")
	out := make([]string, 0, len(Extensions))
	for _, ext := range Extensions {
		if ext == ".json" {
			continue
		}
		out = append(out, base+ext)
	}
	return out
}

// ParseSidecar reads a sidecar file.
func ParseSidecar(raw []byte) (Sidecar, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return Sidecar{SchemaVersion: 1}, nil
	}
	var out Sidecar
	if err := yaml.Unmarshal(raw, &out); err != nil {
		return Sidecar{}, err
	}
	if out.SchemaVersion == 0 {
		out.SchemaVersion = 1
	}
	return out, nil
}
