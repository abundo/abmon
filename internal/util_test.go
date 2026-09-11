package abmon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStructToString(t *testing.T) {
	type sample struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	got := StructToString(sample{Name: "abc", Count: 3})
	if !strings.Contains(got, `"name": "abc"`) || !strings.Contains(got, `"count": 3`) {
		t.Fatalf("StructToString output missing expected fields: %s", got)
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]int{"charlie": 3, "alpha": 1, "bravo": 2}
	got := SortedKeys(m)
	want := []string{"alpha", "bravo", "charlie"}
	if len(got) != len(want) {
		t.Fatalf("SortedKeys length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SortedKeys = %v, want %v", got, want)
		}
	}
}

func TestSortedKeysEmpty(t *testing.T) {
	m := map[string]int{}
	got := SortedKeys(m)
	if len(got) != 0 {
		t.Fatalf("SortedKeys(empty) = %v, want empty", got)
	}
}

func TestFileCmpIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	content := strings.Repeat("hello world\n", 500) // bigger than the 4096 chunk size
	if err := os.WriteFile(f1, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	same, err := FileCmp(f1, f2)
	if err != nil {
		t.Fatalf("FileCmp error: %v", err)
	}
	if !same {
		t.Fatal("FileCmp = false, want true for identical content")
	}
}

func TestFileCmpSameFile(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f1, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	same, err := FileCmp(f1, f1)
	if err != nil {
		t.Fatalf("FileCmp error: %v", err)
	}
	if !same {
		t.Fatal("FileCmp(f, f) = false, want true")
	}
}

func TestFileCmpDifferentSize(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(f1, []byte("short"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("a much longer string"), 0644); err != nil {
		t.Fatal(err)
	}
	same, err := FileCmp(f1, f2)
	if err != nil {
		t.Fatalf("FileCmp error: %v", err)
	}
	if same {
		t.Fatal("FileCmp = true, want false for different sizes")
	}
}

func TestFileCmpDifferentContentSameSize(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(f1, []byte("aaaaa"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("bbbbb"), 0644); err != nil {
		t.Fatal(err)
	}
	same, err := FileCmp(f1, f2)
	if err != nil {
		t.Fatalf("FileCmp error: %v", err)
	}
	if same {
		t.Fatal("FileCmp = true, want false for different content")
	}
}

func TestFileCmpMissingFile(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f1, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := FileCmp(f1, filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("FileCmp with missing file: want error, got nil")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "sub", "dst.txt") // parent dir must already exist for os.Create
	if err := os.WriteFile(src, []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile error: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("copied content = %q, want %q", got, "payload")
	}
}

func TestCopyFileMissingSource(t *testing.T) {
	dir := t.TempDir()
	err := CopyFile(filepath.Join(dir, "missing.txt"), filepath.Join(dir, "dst.txt"))
	if err == nil {
		t.Fatal("CopyFile with missing source: want error, got nil")
	}
}
