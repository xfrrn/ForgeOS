package localrepo

import (
	"reflect"
	"testing"
)

func TestRubyInventoryDistinguishesDeclarationsAndLockSections(t *testing.T) {
	tests := []struct {
		path, manifest, problem string
		deps                    []string
		unresolved              int
	}{
		{"Gemfile", `source "https://example.invalid/#mirror"
group :development, :test do
 gem("my.gem", "~> 2", require: ["main", "extra"]) # gem 'hidden'
end
gem 'another_lib', :require => false
gemspec
eval_gemfile 'extra/Gemfile'
`, "", []string{"another_lib", "my.gem"}, 2},
		{"gems.rb", "gem 'earlier'\ntext = <<~DOC\ngem 'inside-a-string'\nDOC", "dynamic_ruby", nil, 0},
		{"Gemfile", "gem \"#{ENV['TEST']}\"", "dynamic_ruby", nil, 0},
		{"Gemfile", "group :test do\ngem 'unfinished'", "invalid_manifest", nil, 0},
		{"Gemfile.lock", "PATH\n  remote: private/path\n  specs:\n    local-gem (1.2)\n      indirect\nDEPENDENCIES\n  local-gem!\n  rack (>= 2, < 4)\n\nBUNDLED WITH\n   2.6.9\n", "", []string{"local-gem", "rack"}, 0},
		{"gems.locked", "DEPENDENCIES\r\n\r\nBUNDLED WITH\r\n   2.6.9\r\n", "", nil, 0},
		{"Gemfile.lock", "DEPENDENCIES\n  ok\n  https://private.invalid/", "invalid_manifest", nil, 0},
	}
	for _, c := range tests {
		f := InventoryFile{Path: c.path, Kind: inventoryKind(c.path)}
		f.readManifest([]byte(c.manifest))
		if f.Problem != c.problem || !reflect.DeepEqual(c.deps, f.Dependencies) || f.UnresolvedInputs != c.unresolved {
			t.Errorf("%s: got %+v", c.path, f)
		}
	}
}
