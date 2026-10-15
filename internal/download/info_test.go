package download

import (
	"strings"
	"testing"

	"transferit/internal/api"
	"transferit/internal/crypto"
)

func TestRenderInfoListsFilesAndSkipsTheRootFolder(t *testing.T) {
	title := crypto.Base64URLEncode([]byte("Notes"))
	nodes := []api.FileNode{
		{H: "root", P: "", Type: "folder", Name: "Notes"},
		{H: "dir", P: "root", Type: "folder", Name: "photos"},
		{H: "a", P: "dir", Type: "file", Name: "b.jpg", S: 2048},
		{H: "b", P: "dir", Type: "file", Name: "a.jpg", S: 1024},
		{H: "c", P: "root", Type: "file", Name: "readme.txt", S: 12},
	}
	got := renderInfo("AbCdEfGhIjKl", &api.TransferInfo{
		Size: []int64{3084, 3, 2},
		T:    title,
	}, nodes)
	for _, want := range []string{
		"link      https://transfer.it/t/AbCdEfGhIjKl",
		"title     Notes",
		"password  none",
		"photos/",
		"a.jpg",
		"b.jpg",
		"readme.txt",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	if strings.Index(got, "a.jpg") > strings.Index(got, "b.jpg") {
		t.Fatalf("files not sorted:\n%s", got)
	}
	if strings.Contains(got, "\n  Notes/\n") {
		t.Fatalf("root folder should stay out of the list:\n%s", got)
	}
}

func TestRenderInfoHidesTheListUntilAPasswordIsPassed(t *testing.T) {
	got := renderInfo("AbCdEfGhIjKl", &api.TransferInfo{
		Size: []int64{10, 2, 1},
		PW:   []byte("1"),
	}, nil)
	if !strings.Contains(got, "password  required") {
		t.Fatalf("password line:\n%s", got)
	}
	if !strings.Contains(got, "pass -p to list the files") {
		t.Fatalf("hint:\n%s", got)
	}
	if passwordRequired(nil) || passwordRequired([]byte("0")) || passwordRequired([]byte("null")) {
		t.Fatal("empty password flag was treated as protected")
	}
}
