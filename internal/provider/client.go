package provider

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	baseURL = "https://api.zone.eu/v2"
)

// Client represents the Zone.EU API client
type Client struct {
	httpClient *http.Client
	username   string
	apiKey     string
}

// NewClient creates a new Zone.EU API client
func NewClient(username, apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{},
		username:   username,
		apiKey:     apiKey,
	}
}

// authHeader returns the Basic Auth header value
func (c *Client) authHeader() string {
	auth := c.username + ":" + c.apiKey
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

// doRequest performs an HTTP request with authentication
func (c *Client) doRequest(method, path string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("error marshaling request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, baseURL+path, bodyReader)
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
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
	// For SSHFP records
	Algorithm int `json:"algorithm,omitempty"`
	Type      int `json:"type,omitempty"`
	// For URL records
	RedirectType int `json:"redirect_type,omitempty"`
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

func (c *Client) GetARecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/a/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	var record DNSRecord
	if err := json.Unmarshal(resp, &record); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &record, nil
}

func (c *Client) CreateARecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/a", zone), record)
	if err != nil {
		return nil, err
	}
	var created DNSRecord
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &created, nil
}

func (c *Client) UpdateARecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PATCH", fmt.Sprintf("/dns/%s/a/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	var updated DNSRecord
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &updated, nil
}

func (c *Client) DeleteARecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/a/%s", zone, id), nil)
	return err
}

// ==================== AAAA Records ====================

func (c *Client) GetAAAARecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/aaaa/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	var record DNSRecord
	if err := json.Unmarshal(resp, &record); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &record, nil
}

func (c *Client) CreateAAAARecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/aaaa", zone), record)
	if err != nil {
		return nil, err
	}
	var created DNSRecord
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &created, nil
}

func (c *Client) UpdateAAAARecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PATCH", fmt.Sprintf("/dns/%s/aaaa/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	var updated DNSRecord
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &updated, nil
}

func (c *Client) DeleteAAAARecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/aaaa/%s", zone, id), nil)
	return err
}

// ==================== CNAME Records ====================

func (c *Client) GetCNAMERecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/cname/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	var record DNSRecord
	if err := json.Unmarshal(resp, &record); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &record, nil
}

func (c *Client) CreateCNAMERecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/cname", zone), record)
	if err != nil {
		return nil, err
	}
	var created DNSRecord
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &created, nil
}

func (c *Client) UpdateCNAMERecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PATCH", fmt.Sprintf("/dns/%s/cname/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	var updated DNSRecord
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &updated, nil
}

func (c *Client) DeleteCNAMERecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/cname/%s", zone, id), nil)
	return err
}

// ==================== MX Records ====================

func (c *Client) GetMXRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/mx/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	var record DNSRecord
	if err := json.Unmarshal(resp, &record); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &record, nil
}

func (c *Client) CreateMXRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/mx", zone), record)
	if err != nil {
		return nil, err
	}
	var created DNSRecord
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &created, nil
}

func (c *Client) UpdateMXRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PATCH", fmt.Sprintf("/dns/%s/mx/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	var updated DNSRecord
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &updated, nil
}

func (c *Client) DeleteMXRecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/mx/%s", zone, id), nil)
	return err
}

// ==================== TXT Records ====================

func (c *Client) GetTXTRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/txt/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	var record DNSRecord
	if err := json.Unmarshal(resp, &record); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &record, nil
}

func (c *Client) CreateTXTRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/txt", zone), record)
	if err != nil {
		return nil, err
	}
	var created DNSRecord
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &created, nil
}

func (c *Client) UpdateTXTRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PATCH", fmt.Sprintf("/dns/%s/txt/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	var updated DNSRecord
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &updated, nil
}

func (c *Client) DeleteTXTRecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/txt/%s", zone, id), nil)
	return err
}

// ==================== NS Records ====================

func (c *Client) GetNSRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/ns/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	var record DNSRecord
	if err := json.Unmarshal(resp, &record); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &record, nil
}

func (c *Client) CreateNSRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/ns", zone), record)
	if err != nil {
		return nil, err
	}
	var created DNSRecord
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &created, nil
}

func (c *Client) UpdateNSRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PATCH", fmt.Sprintf("/dns/%s/ns/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	var updated DNSRecord
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &updated, nil
}

func (c *Client) DeleteNSRecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/ns/%s", zone, id), nil)
	return err
}

// ==================== SRV Records ====================

func (c *Client) GetSRVRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/srv/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	var record DNSRecord
	if err := json.Unmarshal(resp, &record); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &record, nil
}

func (c *Client) CreateSRVRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/srv", zone), record)
	if err != nil {
		return nil, err
	}
	var created DNSRecord
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &created, nil
}

func (c *Client) UpdateSRVRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PATCH", fmt.Sprintf("/dns/%s/srv/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	var updated DNSRecord
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &updated, nil
}

func (c *Client) DeleteSRVRecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/srv/%s", zone, id), nil)
	return err
}

// ==================== CAA Records ====================

func (c *Client) GetCAARecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/caa/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	var record DNSRecord
	if err := json.Unmarshal(resp, &record); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &record, nil
}

func (c *Client) CreateCAARecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/caa", zone), record)
	if err != nil {
		return nil, err
	}
	var created DNSRecord
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &created, nil
}

func (c *Client) UpdateCAARecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PATCH", fmt.Sprintf("/dns/%s/caa/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	var updated DNSRecord
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &updated, nil
}

func (c *Client) DeleteCAARecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/caa/%s", zone, id), nil)
	return err
}

// ==================== TLSA Records ====================

func (c *Client) GetTLSARecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/tlsa/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	var record DNSRecord
	if err := json.Unmarshal(resp, &record); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &record, nil
}

func (c *Client) CreateTLSARecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/tlsa", zone), record)
	if err != nil {
		return nil, err
	}
	var created DNSRecord
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &created, nil
}

func (c *Client) UpdateTLSARecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PATCH", fmt.Sprintf("/dns/%s/tlsa/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	var updated DNSRecord
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &updated, nil
}

func (c *Client) DeleteTLSARecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/tlsa/%s", zone, id), nil)
	return err
}

// ==================== SSHFP Records ====================

func (c *Client) GetSSHFPRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/sshfp/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	var record DNSRecord
	if err := json.Unmarshal(resp, &record); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &record, nil
}

func (c *Client) CreateSSHFPRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/sshfp", zone), record)
	if err != nil {
		return nil, err
	}
	var created DNSRecord
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &created, nil
}

func (c *Client) UpdateSSHFPRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PATCH", fmt.Sprintf("/dns/%s/sshfp/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	var updated DNSRecord
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &updated, nil
}

func (c *Client) DeleteSSHFPRecord(zone, id string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/dns/%s/sshfp/%s", zone, id), nil)
	return err
}

// ==================== URL Records ====================

func (c *Client) GetURLRecord(zone, id string) (*DNSRecord, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/dns/%s/url/%s", zone, id), nil)
	if err != nil {
		return nil, err
	}
	var record DNSRecord
	if err := json.Unmarshal(resp, &record); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &record, nil
}

func (c *Client) CreateURLRecord(zone string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("POST", fmt.Sprintf("/dns/%s/url", zone), record)
	if err != nil {
		return nil, err
	}
	var created DNSRecord
	if err := json.Unmarshal(resp, &created); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &created, nil
}

func (c *Client) UpdateURLRecord(zone, id string, record *DNSRecord) (*DNSRecord, error) {
	resp, err := c.doRequest("PATCH", fmt.Sprintf("/dns/%s/url/%s", zone, id), record)
	if err != nil {
		return nil, err
	}
	var updated DNSRecord
	if err := json.Unmarshal(resp, &updated); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &updated, nil
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
	var dnsZone DNSZone
	if err := json.Unmarshal(resp, &dnsZone); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &dnsZone, nil
}
