package ccverify

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.commandcode.ai"

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (c *Client) Verify(key string) error {
	base := strings.TrimRight(c.BaseURL, "/")
	url := base + "/alpha/whoami"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("não consegui falar com a API Command Code em %s: %v. O switch local não depende disso.", url, err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return fmt.Errorf("não consegui falar com a API Command Code em %s: %v. O switch local não depende disso.", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("a chave ativa foi recusada pela API Command Code (HTTP %d). Troque de conta ou gere outra chave.", resp.StatusCode)
	}
	return fmt.Errorf("a API Command Code respondeu HTTP %d ao verificar a chave ativa.", resp.StatusCode)
}
