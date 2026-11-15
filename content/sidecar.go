package content

import (
	"path"
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
