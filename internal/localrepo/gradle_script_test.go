package localrepo

import (
	"reflect"
	"testing"
)

func TestGradleDirectConfigurationsExcludeNestedAndDynamicSources(t *testing.T) {
	const body = `plugins { id("java") }
 val example = """dependencies { api("ignored:string:1") }"""
 /* dependencies { implementation("ignored:comment:1") } */
 dependencies {
   implementation(
     "example:main:1"
   ) { exclude(group = "example", module = "transitive") }
   add("custom", libs.bundles.core.kit)
   testImplementation(platform("example:bom:2"))
   compileOnly(project(":support"))
   runtimeOnly(group="example", name="tool", version=someVersion)
   implementation(providerValue)
   constraints { implementation("ignored:constraint:1") }
   if (flag)
       implementation("ignored:conditional:1")
 }
 subprojects { dependencies { implementation("ignored:child:1") } }
 `
	want := map[string][]string{"implementation": {"example:main"}, "custom": {"libs.bundles.core.kit"}, "testImplementation": {"platform(example:bom)"}, "compileOnly": {"project(:support)"}, "runtimeOnly": {"example:tool"}}
	f := InventoryFile{Path: "build.gradle.kts", Kind: "gradle"}
	f.readManifest([]byte(body))
	if f.Problem != "" || f.UnresolvedInputs != 4 || !reflect.DeepEqual(f.GradleConfigurations, want) {
		t.Fatal(f)
	}
}
func TestGradleStringBoundariesAndGroovyMapNotation(t *testing.T) {
	for _, c := range []struct {
		file, script, problem string
		expected              map[string][]string
	}{
		{"build.gradle", "dependencies { implementation group: 'org.acme', name: 'lib', version: '2'; testImplementation 'org.test:mock:3' }", "", map[string][]string{"implementation": {"org.acme:lib"}, "testImplementation": {"org.test:mock"}}},
		{"build.gradle", "def example = /dependencies { api('not:code') }/", "dynamic_gradle", nil},
		{"build.gradle", "dependencies { implementation 'real:module:1'", "invalid_manifest", nil},
		{"build.gradle.kts", "plugins { " + string(rune(96)) + "java-library" + string(rune(96)) + " }; /* outer /* inner */ dependencies { api(\"not:code\") } */ dependencies { api(\"example:real:1\") }", "", map[string][]string{"api": {"example:real"}}},
	} {
		f := InventoryFile{Path: c.file, Kind: "gradle"}
		f.readManifest([]byte(c.script))
		if f.Problem != c.problem || !reflect.DeepEqual(f.GradleConfigurations, c.expected) {
			t.Fatal(f)
		}
	}
}
