package localrepo

import (
	"reflect"
	"strings"
	"testing"
)

func TestMavenInheritanceAndProfiles(t *testing.T) {
	const header = "<project><modelVersion>4.0.0</modelVersion>"
	parent := readMaven([]byte(header+`<groupId> root </groupId><artifactId>parent</artifactId><version>1</version>
<properties><family>parent</family></properties>
<dependencyManagement><dependencies>
<dependency><groupId>managed</groupId><artifactId>only</artifactId></dependency>
<dependency><groupId>framework</groupId><artifactId>platform</artifactId><type>pom</type><scope>import</scope></dependency>
</dependencies></dependencyManagement></project>`), "parent-blob")
	const childXML = header + `<parent><groupId>root</groupId><artifactId>parent</artifactId><version>1</version><relativePath>../parent</relativePath></parent>
<artifactId>app</artifactId><properties><family>child</family><cycle>${cycle}</cycle></properties>
<dependencies>
<dependency><groupId>${family}</groupId><artifactId>${project.artifactId}</artifactId></dependency>
<dependency><groupId>${cycle}</groupId><artifactId>never</artifactId></dependency>
<dependency><groupId>${env.HOME}</groupId><artifactId>never</artifactId></dependency>
</dependencies>
<profiles><profile><id> test </id><properties><family>profile</family></properties><dependencies>
<dependency><groupId>${family}</groupId><artifactId>extra</artifactId></dependency>
</dependencies></profile></profiles></project>`
	check := func(xml string, budget int) InventoryFile {
		t.Helper()
		child := readMaven([]byte(xml), "child-blob")
		if parent == nil || child == nil {
			t.Fatal("valid POM rejected")
		}
		f := InventoryFile{Path: "app/pom.xml"}
		f.resolveMaven(child, map[string]*mavenPOM{"parent/pom.xml": parent}, &budget)
		return f
	}
	f := check(childXML, 1<<20)
	if !reflect.DeepEqual(f.ParentSources, []ManifestSource{{Path: "parent/pom.xml", Blob: "parent-blob"}}) ||
		!reflect.DeepEqual(f.Dependencies, []string{"child:app"}) || !reflect.DeepEqual(f.BOMImports, []string{"framework:platform"}) ||
		!reflect.DeepEqual(f.ManagedDependencies, []string{"managed:only"}) || !reflect.DeepEqual(f.ProfileDependencies["test"].Dependencies, []string{"profile:extra"}) ||
		f.UnresolvedInputs != 2 || f.Problem != "" {
		t.Fatalf("incorrect resolved declarations: %+v", f)
	}
	for _, child := range []string{
		strings.Replace(childXML, "../parent", "", 1),
		strings.Replace(childXML, "../parent", "../../outside", 1),
		strings.Replace(childXML, "<version>1", "<version>2", 1),
	} {
		result := check(child, 1<<20)
		if result.UnresolvedInputs != 3 || len(result.ParentSources) != 0 || len(result.ManagedDependencies) != 0 {
			t.Fatalf("unavailable parent attributed: %+v", result)
		}
	}
	for _, budget := range []int{0, parent.size + 1} {
		if result := check(childXML, budget); result.Problem != "analysis_budget" {
			t.Fatalf("analysis budget ignored: %+v", result)
		}
	}
	for _, xml := range []string{childXML + "<project/>", "<project/>", strings.TrimSuffix(childXML, "</project>")} {
		if readMaven([]byte(xml), "blob") != nil {
			t.Fatal("invalid POM accepted")
		}
	}
}
