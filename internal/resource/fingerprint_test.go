package resource

import "testing"

func TestFingerprintStable(t *testing.T) {
	r := &Resource{Name: "a", Kind: KindLocal, Local: &LocalSpec{Cmd: "run"}}
	first, second := r.Fingerprint(), r.Fingerprint()
	if first != second {
		t.Error("fingerprint must be stable for identical resource")
	}
}

func TestFingerprintChangesWithCmd(t *testing.T) {
	a := &Resource{Name: "a", Kind: KindLocal, Local: &LocalSpec{Cmd: "run-v1"}}
	b := &Resource{Name: "a", Kind: KindLocal, Local: &LocalSpec{Cmd: "run-v2"}}
	if a.Fingerprint() == b.Fingerprint() {
		t.Error("changing cmd should change fingerprint")
	}
}

func TestFingerprintChangesWithDeps(t *testing.T) {
	a := &Resource{Name: "a", Kind: KindLocal, Local: &LocalSpec{Cmd: "run"}}
	b := &Resource{Name: "a", Kind: KindLocal, Local: &LocalSpec{Cmd: "run"}, Deps: []string{"db"}}
	if a.Fingerprint() == b.Fingerprint() {
		t.Error("changing deps should change fingerprint")
	}
}

func TestDiffStacks(t *testing.T) {
	old := &Stack{Name: "s", Resources: []*Resource{
		{Name: "keep", Kind: KindLocal, Local: &LocalSpec{Cmd: "same"}},
		{Name: "change", Kind: KindLocal, Local: &LocalSpec{Cmd: "v1"}},
		{Name: "remove", Kind: KindLocal, Local: &LocalSpec{Cmd: "gone"}},
	}}
	next := &Stack{Name: "s", Resources: []*Resource{
		{Name: "keep", Kind: KindLocal, Local: &LocalSpec{Cmd: "same"}},
		{Name: "change", Kind: KindLocal, Local: &LocalSpec{Cmd: "v2"}},
		{Name: "add", Kind: KindLocal, Local: &LocalSpec{Cmd: "new"}},
	}}
	d := DiffStacks(old, next)
	if len(d.Added) != 1 || d.Added[0] != "add" {
		t.Errorf("added wrong: %v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0] != "remove" {
		t.Errorf("removed wrong: %v", d.Removed)
	}
	if len(d.Changed) != 1 || d.Changed[0] != "change" {
		t.Errorf("changed wrong: %v", d.Changed)
	}
}

func TestDiffStacksNoChange(t *testing.T) {
	s := func() *Stack {
		return &Stack{Name: "s", Resources: []*Resource{
			{Name: "a", Kind: KindLocal, Local: &LocalSpec{Cmd: "x"}},
		}}
	}
	d := DiffStacks(s(), s())
	if len(d.Added)+len(d.Removed)+len(d.Changed) != 0 {
		t.Errorf("identical stacks should have empty diff: %+v", d)
	}
}
