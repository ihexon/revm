package filesystem

import "testing"

func TestParseMounts(t *testing.T) {
	for _, values := range [][]string{
		{"/tmp:/work,readonly"}, {"/tmp:/work,rw"}, {"/tmp:/work,ro,rw"},
		{"/tmp:/work,"}, {"/tmp"}, {":/work"}, {"/tmp:relative"}, {""},
		{"/tmp:/work", "/tmp:/work/../work"},
	} {
		if _, err := ParseMounts(values); err == nil {
			t.Errorf("accepted invalid mounts %q", values)
		}
	}
	mounts, err := ParseMounts([]string{"/tmp:/work,ro", "/tmp:/other"})
	if err != nil {
		t.Fatal(err)
	}
	if !mounts[0].ReadOnly || mounts[1].ReadOnly || mounts[0].Tag == mounts[1].Tag {
		t.Fatalf("unexpected mounts: %+v", mounts)
	}
}
