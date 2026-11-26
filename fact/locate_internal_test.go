package fact

import (
	"testing"

	"github.com/norcubeplatform/cms-schema/report"
)

// Every kind of declaration has a folder and a config key the locator
// looks in, each its own, and the locator knows no other kind.
func TestEveryDeclarationKindHasAPlace(t *testing.T) {
	dirs, keys := map[string]bool{}, map[string]bool{}
	for _, k := range report.DeclarationKinds() {
		p, ok := declarationPlaces[k]
		if !ok || p.dir == "" || p.key == "" {
			t.Errorf("declaration kind %q has no place: %+v", k, p)
			continue
		}
		if dirs[p.dir] || keys[p.key] {
			t.Errorf("declaration kind %q shares %s or %s with another", k, p.dir, p.key)
		}
		dirs[p.dir], keys[p.key] = true, true
	}
	if len(declarationPlaces) != len(report.DeclarationKinds()) {
		t.Errorf("%d places for %d declaration kinds", len(declarationPlaces), len(report.DeclarationKinds()))
	}
}
