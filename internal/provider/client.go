package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	baseURL = "https://api.zone.eu/v2"

	// Rate limiting constants
	defaultRateLimit     = 60 // requests per minute
	rateLimitResetPeriod = time.Minute
	maxRetries           = 3
	retryBaseDelay       = time.Second
)

// Client represents the Zone.EU API client
type Client struct {
	httpClient *http.Client
	username   string
	apiKey     string

	// Rate limiting
	mu                 sync.Mutex
	rateLimitLimit     int
	rateLimitRemaining int
	rateLimitResetAt   time.Time
}

// NewClient creates a new Zone.EU API client
func NewClient(username, apiKey string) *Client {
	return &Client{
		httpClient:         &http.Client{Timeout: 30 * time.Second},
		username:           username,
		apiKey:             apiKey,
		rateLimitLimit:     defaultRateLimit,
		rateLimitRemaining: defaultRateLimit,
	}
}

// authHeader returns the Basic Auth header value
func (c *Client) authHeader() string {
	auth := c.username + ":" + c.apiKey
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

// parseDNSRecordResponse parses the API response which always returns an array
// and extracts the first element
func parseDNSRecordResponse(resp []byte) (*DNSRecord, error) {
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}
	return &records[0], nil
}

// parseDNSZoneResponse parses the API response which always returns an array
// and extracts the first element
func parseDNSZoneResponse(resp []byte) (*DNSZone, error) {
	var zones []DNSZone
	if err := json.Unmarshal(resp, &zones); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	if len(zones) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}
	return &zones[0], nil
}

// updateRateLimitInfo updates rate limit info from response headers
func (c *Client) updateRateLimitInfo(resp *http.Response) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if limit := resp.Header.Get("X-Ratelimit-Limit"); limit != "" {
		if val, err := strconv.Atoi(limit); err == nil {
			c.rateLimitLimit = val
		}
	}

	if remaining := resp.Header.Get("X-Ratelimit-Remaining"); remaining != "" {
		if val, err := strconv.Atoi(remaining); err == nil {
			c.rateLimitRemaining = val
		}
	}
}

// waitForRateLimit waits if we've hit the rate limit
func (c *Client) waitForRateLimit() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If we have remaining requests, no need to wait
	if c.rateLimitRemaining > 0 {
		return
	}

	// Calculate wait time until reset
	now := time.Now()
	if c.rateLimitResetAt.After(now) {
		waitDuration := c.rateLimitResetAt.Sub(now)
		c.mu.Unlock()
		time.Sleep(waitDuration)
		c.mu.Lock()
	}
}

// doRequest performs an HTTP request with authentication and rate limiting
// Uses context.Background() for backward compatibility - prefer doRequestWithContext for new code
func (c *Client) doRequest(method, path string, body interface{}) ([]byte, error) {
	return c.doRequestWithContext(context.Background(), method, path, body)
}

// doRequestWithContext performs an HTTP request with authentication, rate limiting, and context support
func (c *Client) doRequestWithContext(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Wait if we've hit rate limit
		c.waitForRateLimit()

		result, err := c.doRequestOnce(ctx, method, path, body)
		if err == nil {
			return result, nil
		}

		// Check if it's a rate limit error
		if rateLimitErr, ok := err.(*RateLimitError); ok {
			// Set reset time and wait
			c.mu.Lock()
			c.rateLimitRemaining = 0
			c.rateLimitResetAt = time.Now().Add(rateLimitErr.RetryAfter)
			c.mu.Unlock()

			lastErr = err
			time.Sleep(rateLimitErr.RetryAfter)
			continue
		}

		// For other errors, return immediately
		return nil, err
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// RateLimitError represents a rate limit error from the API
type RateLimitError struct {
	RetryAfter time.Duration
	Message    string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded, retry after %v: %s", e.RetryAfter, e.Message)
}

// doRequestOnce performs a single HTTP request
func (c *Client) doRequestOnce(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("error marshaling request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error performing request: %w", err)
	}
	defer resp.Body.Close()

	// Update rate limit info from headers
	c.updateRateLimitInfo(resp)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	// Handle rate limiting (429 Too Many Requests)
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := rateLimitResetPeriod // default to 1 minute
		if retryHeader := resp.Header.Get("Retry-After"); retryHeader != "" {
			if seconds, err := strconv.Atoi(retryHeader); err == nil {
				retryAfter = time.Duration(seconds) * time.Second
			}
		}
		statusMsg := resp.Header.Get("X-Status-Message")
		return nil, &RateLimitError{
			RetryAfter: retryAfter,
			Message:    statusMsg,
		}
	}

	// Handle other error status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		statusMsg := resp.Header.Get("X-Status-Message")
		errMsg := string(respBody)
		if statusMsg != "" {
			errMsg = fmt.Sprintf("%s (X-Status-Message: %s)", errMsg, statusMsg)
		}
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, errMsg)
	}

	return respBody, nil
}

// DNSRecord represents a generic DNS record
type DNSRecord struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Destination string `json:"destination"`
	// For MX records
	Priority int `json:"priority,omitempty"`
	// For SRV records
	Weight int `json:"weight,omitempty"`
	Port   int `json:"port,omitempty"`
	// For CAA records
	Flag int    `json:"flag,omitempty"`
	Tag  string `json:"tag,omitempty"`
	// For TLSA records
	CertificateUsage int `json:"certificate_usage,omitempty"`
	Selector         int `json:"selector,omitempty"`
	MatchingType     int `json:"matching_type,omitempty"`
	// For SSHFP records (algorithm, type) and URL records (type=redirect code)
	Algorithm int `json:"algorithm,omitempty"`
	Type      int `json:"type,omitempty"`
}

// DNSZone represents a DNS zone
type DNSZone struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
	IPv6   bool   `json:"ipv6"`
}

// GetZone retrieves zone information
func (c *Client) GetZone(zone string) (*DNSZone, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s", zone), nil)
	if err != nil {
		return nil, err
	}
	var z DNSZone
	if err := json.Unmarshal(resp, &z); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &z, nil
}

// ==================== A Records ====================

// ListARecords retrieves all A records for a zone
func (c *Client) ListARecords(zone string) ([]DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/a", zone), nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return records, nil
}

// FindARecordByName finds an A record by name in a zone
func (c *Client) FindARecordByName(zone, name string) (*DNSRecord, error) {
	records, err := c.ListARecords(zone)
	if err != nil {
		return nil, err
	}
	
	// Normalize the search name - strip zone suffix if present
	zoneSuffix := "." + zone
	searchName := strings.TrimSuffix(name, zoneSuffix)
	
	for _, r := range records {
		// Normalize the record name as well
		recordName := strings.TrimSuffix(r.Name, zoneSuffix)
		if recordName == searchName || r.Name == name {
			return &r, nil
		}
	}
	return nil, nil // Not found
}

func (c *Client) GetARecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/a/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) CreateARecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/a", zone), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) UpdateARecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/dns/%s/a/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) DeleteARecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/a/%s", zone, id), nil)
	return err
}

// ==================== AAAA Records ====================

// ListAAAARecords retrieves all AAAA records for a zone
func (c *Client) ListAAAARecords(zone string) ([]DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/aaaa", zone), nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return records, nil
}

// FindAAAARecordByName finds an AAAA record by name in a zone
func (c *Client) FindAAAARecordByName(zone, name string) (*DNSRecord, error) {
	records, err := c.ListAAAARecords(zone)
	if err != nil {
		return nil, err
	}
	
	// Normalize the search name - strip zone suffix if present
	zoneSuffix := "." + zone
	searchName := strings.TrimSuffix(name, zoneSuffix)
	
	for _, r := range records {
		// Normalize the record name as well
		recordName := strings.TrimSuffix(r.Name, zoneSuffix)
		if recordName == searchName || r.Name == name {
			return &r, nil
		}
	}
	return nil, nil // Not found
}

func (c *Client) GetAAAARecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/aaaa/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) CreateAAAARecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/aaaa", zone), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) UpdateAAAARecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/dns/%s/aaaa/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) DeleteAAAARecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/aaaa/%s", zone, id), nil)
	return err
}

// ==================== CNAME Records ====================

// ListCNAMERecords retrieves all CNAME records for a zone
func (c *Client) ListCNAMERecords(zone string) ([]DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/cname", zone), nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return records, nil
}

// FindCNAMERecordByName finds a CNAME record by name in a zone
func (c *Client) FindCNAMERecordByName(zone, name string) (*DNSRecord, error) {
	records, err := c.ListCNAMERecords(zone)
	if err != nil {
		return nil, err
	}
	
	// Normalize the search name - strip zone suffix if present
	zoneSuffix := "." + zone
	searchName := strings.TrimSuffix(name, zoneSuffix)
	
	for _, r := range records {
		// Normalize the record name as well
		recordName := strings.TrimSuffix(r.Name, zoneSuffix)
		if recordName == searchName || r.Name == name {
			return &r, nil
		}
	}
	return nil, nil // Not found
}

func (c *Client) GetCNAMERecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/cname/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) CreateCNAMERecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/cname", zone), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) UpdateCNAMERecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/dns/%s/cname/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) DeleteCNAMERecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/cname/%s", zone, id), nil)
	return err
}

// ==================== MX Records ====================

// ListMXRecords retrieves all MX records for a zone
func (c *Client) ListMXRecords(zone string) ([]DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/mx", zone), nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return records, nil
}

// FindMXRecordByName finds an MX record by name in a zone
func (c *Client) FindMXRecordByName(zone, name string) (*DNSRecord, error) {
	records, err := c.ListMXRecords(zone)
	if err != nil {
		return nil, err
	}
	
	// Normalize the search name - strip zone suffix if present
	zoneSuffix := "." + zone
	searchName := strings.TrimSuffix(name, zoneSuffix)
	
	for _, r := range records {
		// Normalize the record name as well
		recordName := strings.TrimSuffix(r.Name, zoneSuffix)
		if recordName == searchName || r.Name == name {
			return &r, nil
		}
	}
	return nil, nil // Not found
}

func (c *Client) GetMXRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/mx/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) CreateMXRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/mx", zone), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) UpdateMXRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/dns/%s/mx/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) DeleteMXRecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/mx/%s", zone, id), nil)
	return err
}

// ==================== TXT Records ====================

// ListTXTRecords retrieves all TXT records for a zone
func (c *Client) ListTXTRecords(zone string) ([]DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/txt", zone), nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return records, nil
}

// FindTXTRecordByName finds a TXT record by name in a zone
func (c *Client) FindTXTRecordByName(zone, name string) (*DNSRecord, error) {
	records, err := c.ListTXTRecords(zone)
	if err != nil {
		return nil, err
	}
	
	// Normalize the search name - strip zone suffix if present
	zoneSuffix := "." + zone
	searchName := strings.TrimSuffix(name, zoneSuffix)
	
	for _, r := range records {
		// Normalize the record name as well
		recordName := strings.TrimSuffix(r.Name, zoneSuffix)
		if recordName == searchName || r.Name == name {
			return &r, nil
		}
	}
	return nil, nil // Not found
}

func (c *Client) GetTXTRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/txt/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) CreateTXTRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/txt", zone), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) UpdateTXTRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/dns/%s/txt/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) DeleteTXTRecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/txt/%s", zone, id), nil)
	return err
}

// ==================== NS Records ====================

// ListNSRecords retrieves all NS records for a zone
func (c *Client) ListNSRecords(zone string) ([]DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/ns", zone), nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return records, nil
}

// FindNSRecordByName finds an NS record by name in a zone
func (c *Client) FindNSRecordByName(zone, name string) (*DNSRecord, error) {
	records, err := c.ListNSRecords(zone)
	if err != nil {
		return nil, err
	}
	
	// Normalize the search name - strip zone suffix if present
	zoneSuffix := "." + zone
	searchName := strings.TrimSuffix(name, zoneSuffix)
	
	for _, r := range records {
		// Normalize the record name as well
		recordName := strings.TrimSuffix(r.Name, zoneSuffix)
		if recordName == searchName || r.Name == name {
			return &r, nil
		}
	}
	return nil, nil // Not found
}

func (c *Client) GetNSRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/ns/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) CreateNSRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/ns", zone), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) UpdateNSRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/dns/%s/ns/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) DeleteNSRecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/ns/%s", zone, id), nil)
	return err
}

// ==================== SRV Records ====================

// ListSRVRecords retrieves all SRV records for a zone
func (c *Client) ListSRVRecords(zone string) ([]DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/srv", zone), nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return records, nil
}

// FindSRVRecordByName finds an SRV record by name in a zone
func (c *Client) FindSRVRecordByName(zone, name string) (*DNSRecord, error) {
	records, err := c.ListSRVRecords(zone)
	if err != nil {
		return nil, err
	}
	
	// Normalize the search name - strip zone suffix if present
	zoneSuffix := "." + zone
	searchName := strings.TrimSuffix(name, zoneSuffix)
	
	for _, r := range records {
		// Normalize the record name as well
		recordName := strings.TrimSuffix(r.Name, zoneSuffix)
		if recordName == searchName || r.Name == name {
			return &r, nil
		}
	}
	return nil, nil // Not found
}

func (c *Client) GetSRVRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/srv/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) CreateSRVRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/srv", zone), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) UpdateSRVRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/dns/%s/srv/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) DeleteSRVRecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/srv/%s", zone, id), nil)
	return err
}

// ==================== CAA Records ====================

// ListCAARecords retrieves all CAA records for a zone
func (c *Client) ListCAARecords(zone string) ([]DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/caa", zone), nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return records, nil
}

// FindCAARecordByName finds a CAA record by name in a zone
func (c *Client) FindCAARecordByName(zone, name string) (*DNSRecord, error) {
	records, err := c.ListCAARecords(zone)
	if err != nil {
		return nil, err
	}
	
	// Normalize the search name - strip zone suffix if present
	zoneSuffix := "." + zone
	searchName := strings.TrimSuffix(name, zoneSuffix)
	
	for _, r := range records {
		// Normalize the record name as well
		recordName := strings.TrimSuffix(r.Name, zoneSuffix)
		if recordName == searchName || r.Name == name {
			return &r, nil
		}
	}
	return nil, nil // Not found
}

func (c *Client) GetCAARecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/caa/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) CreateCAARecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/caa", zone), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) UpdateCAARecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/dns/%s/caa/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) DeleteCAARecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/caa/%s", zone, id), nil)
	return err
}

// ==================== TLSA Records ====================

// ListTLSARecords retrieves all TLSA records for a zone
func (c *Client) ListTLSARecords(zone string) ([]DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/tlsa", zone), nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return records, nil
}

// FindTLSARecordByName finds a TLSA record by name in a zone
func (c *Client) FindTLSARecordByName(zone, name string) (*DNSRecord, error) {
	records, err := c.ListTLSARecords(zone)
	if err != nil {
		return nil, err
	}
	
	// Normalize the search name - strip zone suffix if present
	zoneSuffix := "." + zone
	searchName := strings.TrimSuffix(name, zoneSuffix)
	
	for _, r := range records {
		// Normalize the record name as well
		recordName := strings.TrimSuffix(r.Name, zoneSuffix)
		if recordName == searchName || r.Name == name {
			return &r, nil
		}
	}
	return nil, nil // Not found
}

func (c *Client) GetTLSARecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/tlsa/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) CreateTLSARecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/tlsa", zone), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) UpdateTLSARecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/dns/%s/tlsa/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) DeleteTLSARecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/tlsa/%s", zone, id), nil)
	return err
}

// ==================== SSHFP Records ====================

// ListSSHFPRecords retrieves all SSHFP records for a zone
func (c *Client) ListSSHFPRecords(zone string) ([]DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/sshfp", zone), nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return records, nil
}

// FindSSHFPRecordByName finds an SSHFP record by name in a zone
func (c *Client) FindSSHFPRecordByName(zone, name string) (*DNSRecord, error) {
	records, err := c.ListSSHFPRecords(zone)
	if err != nil {
		return nil, err
	}
	
	// Normalize the search name - strip zone suffix if present
	zoneSuffix := "." + zone
	searchName := strings.TrimSuffix(name, zoneSuffix)
	
	for _, r := range records {
		// Normalize the record name as well
		recordName := strings.TrimSuffix(r.Name, zoneSuffix)
		if recordName == searchName || r.Name == name {
			return &r, nil
		}
	}
	return nil, nil // Not found
}

func (c *Client) GetSSHFPRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/sshfp/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) CreateSSHFPRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/sshfp", zone), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) UpdateSSHFPRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/dns/%s/sshfp/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) DeleteSSHFPRecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/sshfp/%s", zone, id), nil)
	return err
}

// ==================== URL Records ====================

// ListURLRecords retrieves all URL records for a zone
func (c *Client) ListURLRecords(zone string) ([]DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/url", zone), nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := json.Unmarshal(resp, &records); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return records, nil
}

// FindURLRecordByName finds a URL record by name in a zone
func (c *Client) FindURLRecordByName(zone, name string) (*DNSRecord, error) {
	records, err := c.ListURLRecords(zone)
	if err != nil {
		return nil, err
	}
	
	// Normalize the search name - strip zone suffix if present
	zoneSuffix := "." + zone
	searchName := strings.TrimSuffix(name, zoneSuffix)
	
	for _, r := range records {
		// Normalize the record name as well
		recordName := strings.TrimSuffix(r.Name, zoneSuffix)
		if recordName == searchName || r.Name == name {
			return &r, nil
		}
	}
	return nil, nil // Not found
}

func (c *Client) GetURLRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/url/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) CreateURLRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/url", zone), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) UpdateURLRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/dns/%s/url/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	return parseDNSRecordResponse(resp)
}

func (c *Client) DeleteURLRecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/url/%s", zone, id), nil)
	return err
}

// ==================== DNS Zone ====================

func (c *Client) GetDNSZone(zone string) (*DNSZone, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s", zone), nil)
	if err != nil {
		return nil, err
	}
	return parseDNSZoneResponse(resp)
}

// ==================== Domain Management ====================

// Domain represents a domain in Zone.EU
type Domain struct {
	ResourceURL          string `json:"resource_url,omitempty"`
	Name                 string `json:"name"`
	Delegated            string `json:"delegated,omitempty"`
	Expires              string `json:"expires,omitempty"`
	DNSSEC               bool   `json:"dnssec"`
	Autorenew            bool   `json:"autorenew"`
	RenewOrder           string `json:"renew_order,omitempty"`
	RenewalNotifications bool   `json:"renewal_notifications"`
	HasPendingTrade      *int   `json:"has_pending_trade,omitempty"`
	HasPendingDNSSEC     bool   `json:"has_pending_dnssec,omitempty"`
	Reactivate           bool   `json:"reactivate,omitempty"`
	AuthKeyEnabled       bool   `json:"auth_key_enabled,omitempty"`
	SigningRequired      bool   `json:"signing_required,omitempty"`
	NameserversCustom    bool   `json:"nameservers_custom"`
}

// DomainUpdate represents the updateable fields for a domain
type DomainUpdate struct {
	Autorenew         *bool `json:"autorenew,omitempty"`
	DNSSEC            *bool `json:"dnssec,omitempty"`
	NameserversCustom *bool `json:"nameservers_custom,omitempty"`
}

// DomainPreferences represents domain preferences
type DomainPreferences struct {
	ResourceURL          string `json:"resource_url,omitempty"`
	RenewalNotifications bool   `json:"renewal_notifications"`
}

// DomainNameserver represents a domain nameserver
type DomainNameserver struct {
	ResourceURL string   `json:"resource_url,omitempty"`
	Hostname    string   `json:"hostname"`
	IP          []string `json:"ip,omitempty"`
}

// GetDomains retrieves all domains
func (c *Client) GetDomains() ([]Domain, error) {
	resp, err := c.doRequest("GET", "/domain", nil)
	if err != nil {
		return nil, err
	}
	var domains []Domain
	if err := json.Unmarshal(resp, &domains); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return domains, nil
}

// GetDomain retrieves a specific domain
func (c *Client) GetDomain(name string) (*Domain, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/domain/%s", name), nil)
	if err != nil {
		return nil, err
	}
	// API returns array with single element
	var domains []Domain
	if err := json.Unmarshal(resp, &domains); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("domain not found: %s", name)
	}
	return &domains[0], nil
}

// UpdateDomain updates a domain's settings
func (c *Client) UpdateDomain(name string, update *DomainUpdate) (*Domain, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/domain/%s", name), update)
	if err != nil {
		return nil, err
	}
	var domains []Domain
	if err := json.Unmarshal(resp, &domains); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("domain not found: %s", name)
	}
	return &domains[0], nil
}

// GetDomainPreferences retrieves domain preferences
func (c *Client) GetDomainPreferences(name string) (*DomainPreferences, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/domain/%s/preferences", name), nil)
	if err != nil {
		return nil, err
	}
	var prefs []DomainPreferences
	if err := json.Unmarshal(resp, &prefs); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	if len(prefs) == 0 {
		return nil, fmt.Errorf("domain preferences not found: %s", name)
	}
	return &prefs[0], nil
}

// UpdateDomainPreferences updates domain preferences
func (c *Client) UpdateDomainPreferences(name string, prefs *DomainPreferences) (*DomainPreferences, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/domain/%s/preferences", name), prefs)
	if err != nil {
		return nil, err
	}
	var updated []DomainPreferences
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	if len(updated) == 0 {
		return nil, fmt.Errorf("domain preferences not found: %s", name)
	}
	return &updated[0], nil
}

// GetDomainNameservers retrieves all nameservers for a domain
func (c *Client) GetDomainNameservers(domain string) ([]DomainNameserver, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/domain/%s/nameserver", domain), nil)
	if err != nil {
		return nil, err
	}
	var nameservers []DomainNameserver
	if err := json.Unmarshal(resp, &nameservers); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return nameservers, nil
}

// GetDomainNameserver retrieves a specific nameserver
func (c *Client) GetDomainNameserver(domain, hostname string) (*DomainNameserver, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/domain/%s/nameserver/%s", domain, hostname), nil)
	if err != nil {
		return nil, err
	}
	var nameservers []DomainNameserver
	if err := json.Unmarshal(resp, &nameservers); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	if len(nameservers) == 0 {
		return nil, fmt.Errorf("nameserver not found: %s", hostname)
	}
	return &nameservers[0], nil
}

// CreateDomainNameservers creates nameservers for a domain (replaces all)
func (c *Client) CreateDomainNameservers(domain string, nameservers []DomainNameserver) ([]DomainNameserver, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/domain/%s/nameserver", domain), nameservers)
	if err != nil {
		return nil, err
	}
	var created []DomainNameserver
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return created, nil
}

// UpdateDomainNameserver updates a specific nameserver
func (c *Client) UpdateDomainNameserver(domain, hostname string, ns *DomainNameserver) (*DomainNameserver, error) {
	resp, err := c.doRequest("PUT", fmt.Sprintf("/domain/%s/nameserver/%s", domain, hostname), ns)
	if err != nil {
		return nil, err
	}
	var updated []DomainNameserver
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	if len(updated) == 0 {
		return nil, fmt.Errorf("nameserver not found after update: %s", hostname)
	}
	return &updated[0], nil
}

// DeleteDomainNameserver deletes a nameserver
func (c *Client) DeleteDomainNameserver(domain, hostname string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/domain/%s/nameserver/%s", domain, hostname), nil)
	return err
}
