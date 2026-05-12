package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// ---------------------------------------------------------------------------
// baseFuncMap — tfid
// ---------------------------------------------------------------------------

func TestBaseFuncMap_Tfid(t *testing.T) {
	fm := baseFuncMap()
	fn := fm["tfid"].(func(string) string)
	cases := []struct{ in, want string }{
		{"my-app", "my_app"},
		{"no-hyphens-at-all", "no_hyphens_at_all"},
		{"already_ok", "already_ok"},
		{"", ""},
		{"a-b-c", "a_b_c"},
	}
	for _, tc := range cases {
		if got := fn(tc.in); got != tc.want {
			t.Errorf("tfid(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// baseFuncMap — gcpSAID
// ---------------------------------------------------------------------------

func TestBaseFuncMap_GcpSAID(t *testing.T) {
	fm := baseFuncMap()
	fn := fm["gcpSAID"].(func(string) string)
	cases := []struct{ in, want string }{
		{"my_service", "my-service"},
		{"MyService", "myservice"},
		// after truncation to 30 chars the trailing hyphen is removed by TrimRight
		{"this-name-is-way-too-long-for-gcp-sa", "this-name-is-way-too-long-for"},
		{"trailing-", "trailing"},
	}
	for _, tc := range cases {
		if got := fn(tc.in); got != tc.want {
			t.Errorf("gcpSAID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// RenderFile
// ---------------------------------------------------------------------------

func TestRenderFile_WritesRenderedContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.tf")
	tmpl := `resource "null_resource" "{{.Name}}" {}`
	type data struct{ Name string }

	if err := RenderFile(path, tmpl, data{"my_resource"}, nil); err != nil {
		t.Fatalf("RenderFile: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, `"my_resource"`) {
		t.Errorf("output missing rendered name: %s", content)
	}
	if !strings.Contains(content, FileHeader) {
		t.Errorf("output missing FileHeader: %s", content)
	}
}

func TestRenderFile_PrependFileHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.tf")

	if err := RenderFile(path, `# body`, nil, nil); err != nil {
		t.Fatalf("RenderFile: %v", err)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), FileHeader) {
		t.Errorf("file should start with FileHeader, got: %s", string(b))
	}
}

func TestRenderFile_ExtraFuncMapAvailableInTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.tf")
	extra := template.FuncMap{
		"shout": func(s string) string { return strings.ToUpper(s) },
	}
	if err := RenderFile(path, `{{shout "hello"}}`, nil, extra); err != nil {
		t.Fatalf("RenderFile: %v", err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "HELLO") {
		t.Errorf("extra func not applied: %s", string(b))
	}
}

func TestRenderFile_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	// Nested path that doesn't exist yet.
	path := filepath.Join(dir, "sub", "dir", "out.tf")
	if err := RenderFile(path, `# ok`, nil, nil); err != nil {
		t.Fatalf("RenderFile should create parent dirs: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("file should have been created")
	}
}

func TestRenderFile_ReturnsErrorOnBadTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.tf")
	// Unclosed action is a parse error.
	err := RenderFile(path, `{{.Unclosed`, nil, nil)
	if err == nil {
		t.Error("expected error for invalid template, got nil")
	}
}

func TestRenderFile_TfidFuncAvailable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.tf")
	if err := RenderFile(path, `{{tfid "my-app"}}`, nil, nil); err != nil {
		t.Fatalf("RenderFile: %v", err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "my_app") {
		t.Errorf("tfid not available in template: %s", string(b))
	}
}

// ---------------------------------------------------------------------------
// WriteFiles
// ---------------------------------------------------------------------------

func TestWriteFiles_CreatesAllFiles(t *testing.T) {
	dir := t.TempDir()
	files := []struct{ Name, Tmpl string }{
		{"a.tf", `# a`},
		{"b.tf", `# b`},
		{"sub/c.tf", `# c`},
	}
	if err := WriteFiles(dir, files, nil, nil); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f.Name)); err != nil {
			t.Errorf("missing file %s: %v", f.Name, err)
		}
	}
}

func TestWriteFiles_CreatesOutputDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new_dir")
	if err := WriteFiles(dir, []struct{ Name, Tmpl string }{{"f.tf", `# x`}}, nil, nil); err != nil {
		t.Fatalf("WriteFiles should create output dir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Error("output dir should have been created")
	}
}

func TestWriteFiles_ReturnsErrorOnBadTemplate(t *testing.T) {
	dir := t.TempDir()
	files := []struct{ Name, Tmpl string }{
		{"ok.tf", `# ok`},
		{"bad.tf", `{{.Unclosed`},
	}
	if err := WriteFiles(dir, files, nil, nil); err == nil {
		t.Error("expected error for invalid template, got nil")
	}
}

func TestWriteFiles_PassesDataToTemplates(t *testing.T) {
	dir := t.TempDir()
	type data struct{ AppName string }
	files := []struct{ Name, Tmpl string }{
		{"main.tf", `# app: {{.AppName}}`},
	}
	if err := WriteFiles(dir, files, data{"my-service"}, nil); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "main.tf"))
	if !strings.Contains(string(b), "my-service") {
		t.Errorf("data not passed to template: %s", string(b))
	}
}
