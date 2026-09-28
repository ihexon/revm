package service

import (
	"reflect"
	"testing"
)

func TestReadOnlyExt4MountUsesSingleNoloadOption(t *testing.T) {
	mnt := Mnt{ReadOnly: true, Type: "ext4", UUID: "test-uuid", Target: "/mnt/data", Opts: "discard"}
	args, err := mnt.makeMountCmdline(UUIDAction)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-o", "discard,ro,noload", "-t", "ext4", "UUID=test-uuid", "/mnt/data"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("mount args = %#v, want %#v", args, want)
	}
}

func TestWritableExt4MountKeepsJournalOrdering(t *testing.T) {
	mnt := Mnt{Type: "ext4", UUID: "test-uuid", Target: "/mnt/data", Opts: "discard"}
	args, err := mnt.makeMountCmdline(UUIDAction)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-o", "discard,data=ordered", "-t", "ext4", "UUID=test-uuid", "/mnt/data"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("mount args = %#v, want %#v", args, want)
	}
}
