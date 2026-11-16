package localrepo

import (
	"reflect"
	"testing"
)

func TestPythonManifestsExposeOnlyDeclaredNames(t *testing.T) {
	project := InventoryFile{Kind: inventoryKind("pyproject.toml"), Path: "pyproject.toml"}
	project.readManifest([]byte(`[project]
dependencies = ["HTTP_X[http2]>=0.2; python_version > '3.9'","fixture-lib @ https://example.invalid/private.whl"]
dynamic = ["gui-scripts", "version"]
[project.optional-dependencies]
tests = ["http-x", "pytest"]
[project.scripts]
station-run = "private.module:main"
[build-system]
requires = ["setuptools"]
[dependency-groups]
qa = ["ruff", {include-group="other"}]
other = ["coverage"]
[tool.poetry.group.docs.dependencies]
sphinx = "^8"
[tool.poetry.dependencies]
python = "^3.12"
my-lib = {path="../private-source"}
`))
	if project.Problem != "" || project.UnresolvedInputs != 0 ||
		!reflect.DeepEqual(project.Dependencies, []string{"coverage", "fixture-lib", "http-x", "my-lib", "pytest", "ruff", "setuptools", "sphinx"}) ||
		!reflect.DeepEqual(project.Scripts, []string{"station-run"}) || !reflect.DeepEqual(project.DynamicFields, []string{"gui-scripts"}) {
		t.Fatal(project)
	}
	pip := InventoryFile{Kind: "python", Path: "requirements.txt"}
	pip.readManifest([]byte("One_Pkg==1 \\\r\n --hash=sha256:private\r\nOne.Pkg\t# note\nwheel-pkg @ https://example.invalid/private.whl\n" +
		"--index-url https://private.invalid\n-r other.txt\n-e ../project\nhttps://example.invalid/download.zip\nlocal.whl\n../unresolved\nSYNTHETIC=private\n"))
	if pip.Problem != "" || pip.UnresolvedInputs != 6 || !reflect.DeepEqual(pip.Dependencies, []string{"one-pkg", "wheel-pkg"}) {
		t.Fatal(pip)
	}
	bad := InventoryFile{Kind: "python", Path: "pyproject.toml"}
	bad.readManifest([]byte("[project"))
	if bad.Problem != "invalid_manifest" {
		t.Fatal(bad)
	}
}
