package pds

import (
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestBuildFolderPathMap_Nested(t *testing.T) {
	now := time.Now()
	files := []pdsFile{
		{FileID: "rootA", Name: "项目A", Type: "folder", ParentID: "root"},
		{FileID: "rootA_sub", Name: "文档", Type: "folder", ParentID: "rootA"},
		{FileID: "doc1", Name: "报告.pdf", Type: "file", ParentID: "rootA_sub", UpdatedAt: now},
		{FileID: "doc2", Name: "README.md", Type: "file", ParentID: "rootA", UpdatedAt: now},
		{FileID: "loose", Name: "顶层.md", Type: "file", ParentID: "root", UpdatedAt: now},
	}
	folderPath := buildFolderPathMap(files)

	if got := folderPath["rootA"]; got != "/项目A" {
		t.Errorf("rootA path = %q, want %q", got, "/项目A")
	}
	if got := folderPath["rootA_sub"]; got != "/项目A/文档" {
		t.Errorf("rootA_sub path = %q, want %q", got, "/项目A/文档")
	}
	// Files don't appear in folderPath (only folders do), but fileFolderPath
	// resolves their parent.
	if got := fileFolderPath(files[2], folderPath); got != "/项目A/文档" {
		t.Errorf("doc1 folder_path = %q, want %q", got, "/项目A/文档")
	}
	if got := fileFolderPath(files[3], folderPath); got != "/项目A" {
		t.Errorf("doc2 folder_path = %q, want %q", got, "/项目A")
	}
	// Top-level file: empty ParentID or "root" → no folder prefix.
	if got := fileFolderPath(files[4], folderPath); got != "" {
		t.Errorf("loose folder_path = %q, want \"\"", got)
	}
}

func TestBuildFolderPathMap_CycleSafe(t *testing.T) {
	// Pathological: A's parent is B, B's parent is A. Should not loop.
	files := []pdsFile{
		{FileID: "A", Name: "A", Type: "folder", ParentID: "B"},
		{FileID: "B", Name: "B", Type: "folder", ParentID: "A"},
	}
	folderPath := buildFolderPathMap(files)
	if _, ok := folderPath["A"]; !ok {
		t.Errorf("expected A to be present even on cycle")
	}
	if _, ok := folderPath["B"]; !ok {
		t.Errorf("expected B to be present even on cycle")
	}
}

func TestBuildFolderPathMap_UnknownParentTreatedAsRoot(t *testing.T) {
	// Folder whose parent isn't in the listing (truncated ScanFile).
	files := []pdsFile{
		{FileID: "orphan", Name: "孤儿", Type: "folder", ParentID: "missing-parent"},
	}
	folderPath := buildFolderPathMap(files)
	if got := folderPath["orphan"]; got != "/孤儿" {
		t.Errorf("orphan path = %q, want %q", got, "/孤儿")
	}
}

func TestBuildFolderPathMap_EmptyListing(t *testing.T) {
	if got := buildFolderPathMap(nil); len(got) != 0 {
		t.Errorf("expected empty map for nil input, got %v", got)
	}
}

func TestScopeRootsFromConfig(t *testing.T) {
	cases := []struct {
		name string
		rids []string
		want []string
	}{
		{"drive-level selection", []string{"d1:"}, nil},
		{"bare drive selection", []string{"d1"}, nil},
		{"folder selection", []string{"d1:folderA"}, []string{"folderA"}},
		{"deep folder selection", []string{"d1:deepFolder"}, []string{"deepFolder"}},
		{"drive plus folder keeps folder", []string{"d1:", "d1:folderA"}, []string{"folderA"}},
		{"other drive ignored", []string{"d2:folderX"}, nil},
		{"dedup", []string{"d1:folderA", "d1:folderA"}, []string{"folderA"}},
		{"empty tokens skipped", []string{"", "d1:folderA"}, []string{"folderA"}},
	}
	for _, tc := range cases {
		cfg := &types.DataSourceConfig{ResourceIDs: tc.rids}
		got := ScopeRootsFromConfig(cfg, "d1")
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
				break
			}
		}
	}
	if got := ScopeRootsFromConfig(nil, "d1"); got != nil {
		t.Errorf("nil config: got %v, want nil", got)
	}
}
