package lock_test

import (
	"strings"
	"testing"

	"github.com/promptlock/promptlock/internal/lock"
)

func TestHash_StableUnderTrailingWhitespace(t *testing.T) {
	a := []byte("---\nid: x\nversion: \"0.1.0\"\nmodel: m\n---\n# User\nhi\n")
	b := []byte("---\nid: x   \nversion: \"0.1.0\"   \nmodel: m\t\n---\n# User\nhi\n\n\n")

	ha, err := lock.HashFile(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := lock.HashFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Errorf("expected equal hashes, got %s vs %s", ha, hb)
	}
}

func TestHash_StableUnderCRLF(t *testing.T) {
	lf := []byte("---\nid: x\nversion: \"0.1.0\"\nmodel: m\n---\n# User\nhi\n")
	crlf := []byte("---\r\nid: x\r\nversion: \"0.1.0\"\r\nmodel: m\r\n---\r\n# User\r\nhi\r\n")

	ha, _ := lock.HashFile(lf)
	hb, _ := lock.HashFile(crlf)
	if ha != hb {
		t.Errorf("CRLF should hash same as LF: %s vs %s", ha, hb)
	}
}

func TestHash_StableUnderKeyReorder(t *testing.T) {
	a := []byte("---\nid: x\nversion: \"0.1.0\"\nmodel: m\n---\n# User\nhi\n")
	b := []byte("---\nmodel: m\nid: x\nversion: \"0.1.0\"\n---\n# User\nhi\n")

	ha, _ := lock.HashFile(a)
	hb, _ := lock.HashFile(b)
	if ha != hb {
		t.Errorf("key reordering must not change hash: %s vs %s", ha, hb)
	}
}

func TestHash_ChangesOnSemanticEdit(t *testing.T) {
	a := []byte("---\nid: x\nversion: \"0.1.0\"\nmodel: claude-opus-4-6\n---\n# User\nhi\n")
	b := []byte("---\nid: x\nversion: \"0.1.0\"\nmodel: claude-opus-4-7\n---\n# User\nhi\n")

	ha, _ := lock.HashFile(a)
	hb, _ := lock.HashFile(b)
	if ha == hb {
		t.Errorf("model change must change hash")
	}
	if !strings.HasPrefix(ha, "sha256:") {
		t.Errorf("hash should be sha256:hex, got %s", ha)
	}
}

func TestHash_ChangesOnBodyEdit(t *testing.T) {
	a := []byte("---\nid: x\nversion: \"0.1.0\"\nmodel: m\n---\n# User\nhi\n")
	b := []byte("---\nid: x\nversion: \"0.1.0\"\nmodel: m\n---\n# User\nhello\n")

	ha, _ := lock.HashFile(a)
	hb, _ := lock.HashFile(b)
	if ha == hb {
		t.Errorf("body change must change hash")
	}
}
