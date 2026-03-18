package claude

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

const (
	USAGE_URL       = "https://api.anthropic.com/api/oauth/usage"
	BETA_HEADER     = "oauth-2025-04-20"
	CACHE_TTL       = 60 * time.Second
	REQUEST_TIMEOUT = 10 * time.Second
)

var (
	cacheMu   sync.Mutex
	cacheData interface{}
	cacheTime time.Time
)

func getCredentials() string {
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
		done := make(chan struct{})
		var out []byte
		var err error
		go func() {
			out, err = cmd.Output()
			close(done)
		}()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-done:
			if err == nil && len(out) > 0 {
				raw := string(out)
				// Strip trailing newline
				if len(raw) > 0 && raw[len(raw)-1] == '\n' {
					raw = raw[:len(raw)-1]
				}
				// Try parsing as JSON
				var parsed map[string]interface{}
				if json.Unmarshal([]byte(raw), &parsed) == nil {
					if oauth, ok := parsed["claudeAiOauth"].(map[string]interface{}); ok {
						if token, ok := oauth["accessToken"].(string); ok && token != "" {
							return token
						}
					}
				}
				// Use raw string as token
				if raw != "" {
					return raw
				}
			}
		case <-timer.C:
			cmd.Process.Kill()
		}
	}

	// Fallback: read ~/.claude/.credentials.json
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	credPath := filepath.Join(home, ".claude", ".credentials.json")
	data, err := os.ReadFile(credPath)
	if err != nil {
		return ""
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ""
	}
	if oauth, ok := parsed["claudeAiOauth"].(map[string]interface{}); ok {
		if token, ok := oauth["accessToken"].(string); ok {
			return token
		}
	}
	return ""
}

func FetchPlanUsage() (interface{}, error) {
	cacheMu.Lock()
	if cacheData != nil && time.Since(cacheTime) < CACHE_TTL {
		data := cacheData
		cacheMu.Unlock()
		return data, nil
	}
	cacheMu.Unlock()

	token := getCredentials()
	if token == "" {
		return nil, nil
	}

	client := &http.Client{Timeout: REQUEST_TIMEOUT}
	req, err := http.NewRequest("GET", USAGE_URL, nil)
	if err != nil {
		return nil, nil
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("anthropic-beta", BETA_HEADER)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}

	cacheMu.Lock()
	cacheData = result
	cacheTime = time.Now()
	cacheMu.Unlock()

	return result, nil
}
