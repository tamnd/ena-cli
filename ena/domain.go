package ena

import (
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes ENA as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/ena-cli/ena"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// ena:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone ena binary (see cmd/ena/main.go), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the ENA driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "ena",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "ena",
			Short:  "Read public European Nucleotide Archive (ENA) records.",
			Long: `Read public European Nucleotide Archive (ENA) records.

ena reads from the EMBL-EBI ENA Portal API (1M+ studies, 312M+ sequences) over
plain HTTPS, shapes it into clean records, and prints output that pipes into the
rest of your tools. No API key required.`,
			Site: Host,
			Repo: "https://github.com/tamnd/ena-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// search: keyword search across ENA studies.
	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search ENA studies by keyword",
		Args:    []kit.Arg{{Name: "query", Help: "search keyword or ENA query expression"}}}, searchStudies)

	// study: fetch a single study by accession.
	kit.Handle(app, kit.OpMeta{Name: "study", Group: "read", Single: true,
		Summary: "Fetch an ENA study by accession", URIType: "study", Resolver: true,
		Args: []kit.Arg{{Name: "accession", Help: "study accession (PRJNA…, ERP…, SRP…)"}}}, getStudy)

	// sequences: search ENA sequences by keyword.
	kit.Handle(app, kit.OpMeta{Name: "sequences", Group: "read", List: true,
		Summary: "Search ENA sequences",
		Args:    []kit.Arg{{Name: "query", Help: "search keyword or ENA query expression"}}}, searchSequences)

	// stats: show record counts for each ENA data type.
	kit.Handle(app, kit.OpMeta{Name: "stats", Group: "read", List: true,
		Summary: "Show record counts for each ENA data type"}, getStats)
}

// newClient builds the ENA client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	ecfg := DefaultConfig()
	if cfg.UserAgent != "" {
		ecfg.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		ecfg.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		ecfg.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		ecfg.Timeout = cfg.Timeout
	}
	return NewClientWithConfig(ecfg), nil
}

// --- inputs ---

type searchInput struct {
	Query  string  `kit:"arg"           help:"search keyword or ENA query expression"`
	Limit  int     `kit:"flag,inherit"  help:"max results"`
	Offset int     `kit:"flag"          help:"result offset"`
	Client *Client `kit:"inject"`
}

type studyRef struct {
	Accession string  `kit:"arg"    help:"study accession (PRJNA…, ERP…, SRP…)"`
	Client    *Client `kit:"inject"`
}

type sequencesInput struct {
	Query  string  `kit:"arg"           help:"search keyword or ENA query expression"`
	Limit  int     `kit:"flag,inherit"  help:"max results"`
	Offset int     `kit:"flag"          help:"result offset"`
	Client *Client `kit:"inject"`
}

type statsInput struct {
	Client *Client `kit:"inject"`
}

// Stat is one row in the stats output.
type Stat struct {
	Type  string `json:"type"  kit:"id"`
	Count int    `json:"count"`
}

// --- handlers ---

func searchStudies(ctx context.Context, in searchInput, emit func(*Study) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	studies, err := in.Client.SearchStudies(ctx, in.Query, limit, in.Offset)
	if err != nil {
		return mapErr(err)
	}
	for _, s := range studies {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

func getStudy(ctx context.Context, in studyRef, emit func(*Study) error) error {
	s, err := in.Client.GetStudy(ctx, in.Accession)
	if err != nil {
		return mapErr(err)
	}
	return emit(s)
}

func searchSequences(ctx context.Context, in sequencesInput, emit func(*Sequence) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	seqs, err := in.Client.SearchSequences(ctx, in.Query, limit, in.Offset)
	if err != nil {
		return mapErr(err)
	}
	for _, s := range seqs {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

func getStats(ctx context.Context, in statsInput, emit func(*Stat) error) error {
	types := []string{"study", "sequence", "read_run", "sample"}
	for _, t := range types {
		n, err := in.Client.GetCount(ctx, t)
		if err != nil {
			return mapErr(fmt.Errorf("count %s: %w", t, err))
		}
		if err := emit(&Stat{Type: t, Count: n}); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: pure string functions, no network ---

// Classify turns any accepted input into the canonical (type, id).
// ENA study accessions start with PRJNA, ERP, SRP, DRP, or similar prefixes.
// Any non-empty string is treated as a study accession.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty ENA reference")
	}
	return "study", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "study":
		return "https://www.ebi.ac.uk/ena/browser/view/" + id, nil
	default:
		return "", errs.Usage("ena has no resource type %q", uriType)
	}
}

// --- helpers ---

// mapErr converts a library error into the kit error kind that carries the right
// exit code.
func mapErr(err error) error {
	return err
}
