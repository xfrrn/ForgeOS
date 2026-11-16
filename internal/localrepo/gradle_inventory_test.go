package localrepo

import (
	"reflect"
	"testing"
)

func TestGradleVersionCatalogNamesAndBundleLinks(t *testing.T) {
	data := `[versions]
core-version = "1.0"
[libraries]
core-api = {module="example:api", version.ref="core.version"}
util_lib = {group="example", name="util", version={strictly="2"}}
more = "example:unused:1"
[bundles]
core-kit = ["util_lib", "core-api", "util_lib"]
`
	f := InventoryFile{Path: "sample/gradle/libs.versions.toml", Kind: inventoryKind("sample/gradle/libs.versions.toml")}
	f.readManifest([]byte(data))
	if f.Kind != "gradle-catalog" || f.Problem != "" || f.CatalogLibraries["core.api"] != "example:api" || len(f.CatalogLibraries) != 3 || !reflect.DeepEqual(f.CatalogBundles["core.kit"], []string{"util.lib", "core.api", "util.lib"}) || len(f.Dependencies) != 0 {
		t.Fatal(f)
	}
	for _, source := range []string{`[libraries]
lib = "only-two:parts"`, `[bundles]
sample=["absent"]`, `[libraries]
lib={module="https://invalid/library"}`} {
		f = InventoryFile{Kind: "gradle-catalog"}
		f.readManifest([]byte(source))
		if f.Problem != "invalid_manifest" || len(f.CatalogLibraries) != 0 || len(f.CatalogBundles) != 0 {
			t.Fatal(f)
		}
	}
}
