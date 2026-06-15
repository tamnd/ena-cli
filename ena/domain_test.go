package ena

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The client's
// HTTP behaviour is covered in ena_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "ena" {
		t.Errorf("Scheme = %q, want ena", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "ena" {
		t.Errorf("Identity.Binary = %q, want ena", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"PRJNA449226", "study", "PRJNA449226"},
		{"ERP000958", "study", "ERP000958"},
		{"SRP000958", "study", "SRP000958"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify(\"\") should return an error")
	}
	_, _, err = Domain{}.Classify("   ")
	if err == nil {
		t.Error("Classify(whitespace) should return an error")
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		uriType, id, want string
	}{
		{"study", "PRJNA449226", "https://www.ebi.ac.uk/ena/browser/view/PRJNA449226"},
		{"study", "ERP000958", "https://www.ebi.ac.uk/ena/browser/view/ERP000958"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.uriType, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)",
				tc.uriType, tc.id, got, err, tc.want)
		}
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("page", "foo")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}

// TestHostWiring mounts the driver in a kit Host (the runtime ant drives) and
// checks the round trip: a record mints to its URI, and a bare id resolves back
// to the same URI. The init in domain.go registers the domain, so kit.Open finds it.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	s := &Study{
		ID:    "PRJNA449226",
		Title: "Cancer panel sequencing of FFPE samples",
	}
	u, err := h.Mint(s)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "ena://study/PRJNA449226"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("ena", "ERP000958")
	if err != nil || got.String() != "ena://study/ERP000958" {
		t.Errorf("ResolveOn = (%q, %v), want ena://study/ERP000958", got.String(), err)
	}
}
