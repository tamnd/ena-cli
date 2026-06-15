package ena

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestClient builds a Client pointing at srv with pacing and retries disabled.
func newTestClient(srv *httptest.Server) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0
	cfg.Timeout = 5 * time.Second
	return NewClientWithConfig(cfg)
}

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	cfg.Timeout = 5 * time.Second
	c := NewClientWithConfig(cfg)

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGetCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/count" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("result") != "study" {
			t.Errorf("result = %q, want study", r.URL.Query().Get("result"))
		}
		// ENA count endpoint returns plain text: header line then integer.
		_, _ = fmt.Fprint(w, "count\n1042937\n")
	}))
	defer srv.Close()

	c := newTestClient(srv)
	n, err := c.GetCount(context.Background(), "study")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1042937 {
		t.Errorf("GetCount = %d, want 1042937", n)
	}
}

func TestSearchStudies(t *testing.T) {
	records := []wireStudy{
		{
			StudyAccession: "PRJNA449226",
			StudyTitle:     "Cancer panel sequencing of FFPE samples",
			Description:    "Targeted sequencing study",
			StudyType:      "Whole Genome Sequencing",
			TaxID:          "9606",
			ScientificName: "Homo sapiens",
			FirstPublic:    "2018-04-01",
			LastUpdated:    "2023-01-15",
		},
		{
			StudyAccession: "ERP000958",
			StudyTitle:     "Breast cancer RNA-seq",
			Description:    "RNA sequencing of breast cancer samples",
			StudyType:      "Transcriptome Analysis",
			TaxID:          "9606",
			ScientificName: "Homo sapiens",
			FirstPublic:    "2011-03-20",
			LastUpdated:    "2022-06-10",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("result") != "study" {
			t.Errorf("result = %q, want study", r.URL.Query().Get("result"))
		}
		b, _ := json.Marshal(records)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	studies, err := c.SearchStudies(context.Background(), "cancer", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(studies) != 2 {
		t.Fatalf("len(studies) = %d, want 2", len(studies))
	}
	if studies[0].ID != "PRJNA449226" {
		t.Errorf("studies[0].ID = %q, want PRJNA449226", studies[0].ID)
	}
	if studies[0].Title != "Cancer panel sequencing of FFPE samples" {
		t.Errorf("studies[0].Title = %q", studies[0].Title)
	}
	if studies[1].ID != "ERP000958" {
		t.Errorf("studies[1].ID = %q, want ERP000958", studies[1].ID)
	}
	if studies[1].Title != "Breast cancer RNA-seq" {
		t.Errorf("studies[1].Title = %q", studies[1].Title)
	}
}

func TestSearchSequences(t *testing.T) {
	records := []wireSequence{
		{
			Accession:      "AF125253",
			Description:    "Homo sapiens BRCA1 mRNA, complete cds",
			MolType:        "mRNA",
			Length:         "5711",
			ScientificName: "Homo sapiens",
			TaxID:          "9606",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("result") != "sequence" {
			t.Errorf("result = %q, want sequence", r.URL.Query().Get("result"))
		}
		b, _ := json.Marshal(records)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	seqs, err := c.SearchSequences(context.Background(), "BRCA1", 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(seqs) != 1 {
		t.Fatalf("len(seqs) = %d, want 1", len(seqs))
	}
	s := seqs[0]
	if s.ID != "AF125253" {
		t.Errorf("ID = %q, want AF125253", s.ID)
	}
	if s.Description != "Homo sapiens BRCA1 mRNA, complete cds" {
		t.Errorf("Description = %q", s.Description)
	}
	if s.MolType != "mRNA" {
		t.Errorf("MolType = %q, want mRNA", s.MolType)
	}
	if s.Length != "5711" {
		t.Errorf("Length = %q, want 5711", s.Length)
	}
	if s.Organism != "Homo sapiens" {
		t.Errorf("Organism = %q, want Homo sapiens", s.Organism)
	}
}

func TestGetStudy(t *testing.T) {
	record := wireStudy{
		StudyAccession: "PRJNA449226",
		StudyTitle:     "Cancer panel sequencing of FFPE samples",
		Description:    "Targeted cancer sequencing",
		TaxID:          "9606",
		ScientificName: "Homo sapiens",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if q == "" {
			t.Error("GetStudy should pass an accession query filter")
		}
		b, _ := json.Marshal([]wireStudy{record})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	s, err := c.GetStudy(context.Background(), "PRJNA449226")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "PRJNA449226" {
		t.Errorf("ID = %q, want PRJNA449226", s.ID)
	}
	if s.Title != "Cancer panel sequencing of FFPE samples" {
		t.Errorf("Title = %q", s.Title)
	}
}

func TestGetStudyNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetStudy(context.Background(), "PRJNA000000")
	if err == nil {
		t.Error("GetStudy with empty result should return an error")
	}
}
