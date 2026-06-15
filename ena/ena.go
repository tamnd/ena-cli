// Package ena is the library behind the ena command line:
// the HTTP client, request shaping, and the typed data models for the
// EMBL-EBI European Nucleotide Archive (ENA).
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package ena

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Host is the ENA portal host, used for URI resolution and the domain Hosts list.
const Host = "www.ebi.ac.uk"

// baseURL is the ENA Portal API root every request is built from.
const baseURL = "https://www.ebi.ac.uk/ena/portal/api"

// Config carries the tunable parameters for the ENA client.
type Config struct {
	BaseURL   string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns a Config with sensible defaults for the ENA Portal API.
func DefaultConfig() Config {
	return Config{
		BaseURL:   baseURL,
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
		UserAgent: "ena-cli/0.1.0 (github.com/tamnd/ena-cli)",
	}
}

// Client talks to the ENA Portal API over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client with the default config.
func NewClient() *Client {
	return NewClientWithConfig(DefaultConfig())
}

// NewClientWithConfig returns a Client using the given config.
func NewClientWithConfig(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = baseURL
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Get fetches rawURL and returns the response body, pacing and retrying.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// buildSearchURL constructs a /search endpoint URL.
func (c *Client) buildSearchURL(result, query, fields string, limit, offset int) string {
	u := fmt.Sprintf("%s/search?result=%s&format=json&limit=%d&offset=%d",
		c.cfg.BaseURL, result, limit, offset)
	if fields != "" {
		u += "&fields=" + url.QueryEscape(fields)
	}
	if query != "" {
		u += "&query=" + url.QueryEscape(query)
	}
	return u
}

// --- wire types (unexported) ---

// wireStudy maps the ENA /search?result=study JSON response fields.
type wireStudy struct {
	StudyAccession  string `json:"study_accession"`
	StudyTitle      string `json:"study_title"`
	Description     string `json:"description"`
	StudyType       string `json:"study_type"`
	TaxID           string `json:"tax_id"`
	ScientificName  string `json:"scientific_name"`
	FirstPublic     string `json:"first_public"`
	LastUpdated     string `json:"last_updated"`
	SecondaryAccess string `json:"secondary_study_accession"`
}

// wireSequence maps the ENA /search?result=sequence JSON response fields.
type wireSequence struct {
	Accession      string `json:"accession"`
	Description    string `json:"description"`
	MolType        string `json:"mol_type"`
	Length         string `json:"sequence_length"`
	ScientificName string `json:"scientific_name"`
	TaxID          string `json:"tax_id"`
}

// --- public output types ---

// Study is a single ENA study record.
type Study struct {
	ID              string `json:"id"                        kit:"id"` // study_accession
	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	StudyType       string `json:"study_type,omitempty"`
	TaxID           string `json:"tax_id,omitempty"`
	Organism        string `json:"organism,omitempty"`
	FirstPublic     string `json:"first_public,omitempty"`
	LastUpdated     string `json:"last_updated,omitempty"`
	SecondaryAccess string `json:"secondary_accession,omitempty"`
}

// Sequence is a single ENA sequence record.
type Sequence struct {
	ID          string `json:"id"          kit:"id"` // accession
	Description string `json:"description,omitempty"`
	MolType     string `json:"mol_type,omitempty"`
	Length      string `json:"length,omitempty"`
	Organism    string `json:"organism,omitempty"`
	TaxID       string `json:"tax_id,omitempty"`
}

// --- client methods ---

// GetCount returns the total record count for a result type (study, sequence, read_run, sample, …).
// The /count endpoint returns plain text: first line is "count", second line is the integer.
func (c *Client) GetCount(ctx context.Context, resultType string) (int, error) {
	u := fmt.Sprintf("%s/count?result=%s&dataPortal=ena", c.cfg.BaseURL, resultType)
	body, err := c.Get(ctx, u)
	if err != nil {
		return 0, err
	}
	// Response format:
	// count
	// 312962673
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	// Find last non-empty line that parses as an integer.
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || line == "count" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return 0, fmt.Errorf("count parse %q: %w", line, err)
		}
		return n, nil
	}
	return 0, fmt.Errorf("count: unexpected response: %q", string(body))
}

// SearchStudies searches ENA studies and returns up to limit records starting at offset.
func (c *Client) SearchStudies(ctx context.Context, query string, limit, offset int) ([]*Study, error) {
	if limit <= 0 {
		limit = 10
	}
	const fields = "study_accession,study_title,description,study_type,tax_id,scientific_name,first_public,last_updated,secondary_study_accession"
	u := c.buildSearchURL("study", query, fields, limit, offset)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var ws []wireStudy
	if err := json.Unmarshal(body, &ws); err != nil {
		return nil, fmt.Errorf("study search parse: %w", err)
	}
	out := make([]*Study, 0, len(ws))
	for _, w := range ws {
		out = append(out, studyFromWire(w))
	}
	return out, nil
}

// SearchSequences searches ENA sequences and returns up to limit records starting at offset.
func (c *Client) SearchSequences(ctx context.Context, query string, limit, offset int) ([]*Sequence, error) {
	if limit <= 0 {
		limit = 10
	}
	const fields = "accession,description,mol_type,sequence_length,scientific_name,tax_id"
	u := c.buildSearchURL("sequence", query, fields, limit, offset)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var ws []wireSequence
	if err := json.Unmarshal(body, &ws); err != nil {
		return nil, fmt.Errorf("sequence search parse: %w", err)
	}
	out := make([]*Sequence, 0, len(ws))
	for _, w := range ws {
		out = append(out, sequenceFromWire(w))
	}
	return out, nil
}

// GetStudy fetches a single study by accession (e.g. PRJNA449226, ERP000958).
func (c *Client) GetStudy(ctx context.Context, accession string) (*Study, error) {
	q := fmt.Sprintf(`study_accession="%s"`, accession)
	studies, err := c.SearchStudies(ctx, q, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(studies) == 0 {
		return nil, fmt.Errorf("study %s: not found", accession)
	}
	return studies[0], nil
}

// --- helpers ---

func studyFromWire(w wireStudy) *Study {
	return &Study{
		ID:              w.StudyAccession,
		Title:           w.StudyTitle,
		Description:     w.Description,
		StudyType:       w.StudyType,
		TaxID:           w.TaxID,
		Organism:        w.ScientificName,
		FirstPublic:     w.FirstPublic,
		LastUpdated:     w.LastUpdated,
		SecondaryAccess: w.SecondaryAccess,
	}
}

func sequenceFromWire(w wireSequence) *Sequence {
	return &Sequence{
		ID:          w.Accession,
		Description: w.Description,
		MolType:     w.MolType,
		Length:      w.Length,
		Organism:    w.ScientificName,
		TaxID:       w.TaxID,
	}
}
