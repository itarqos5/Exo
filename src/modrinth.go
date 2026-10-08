package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

const modrinthUA = "exo-mc-scanner/1.0 (local integrity checker)"

// modrinthVersion is the subset of the Modrinth version object we care about.
type modrinthVersion struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
}

// lookupHashes queries Modrinth's batch version_files endpoint for a set of
// SHA-1 hashes. The returned map is keyed by the lowercase sha1 of mods that
// Modrinth recognizes. ok is false if Modrinth could not be reached at all.
func lookupHashes(sha1s []string) (map[string]modrinthVersion, bool) {
	result := make(map[string]modrinthVersion)
	if len(sha1s) == 0 {
		return result, true
	}
	client := &http.Client{Timeout: 20 * time.Second}
	reachedAny := false

	const chunk = 100
	for i := 0; i < len(sha1s); i += chunk {
		end := i + chunk
		if end > len(sha1s) {
			end = len(sha1s)
		}
		body, _ := json.Marshal(map[string]any{
			"hashes":    sha1s[i:end],
			"algorithm": "sha1",
		})
		req, err := http.NewRequest("POST", "https://api.modrinth.com/v2/version_files", bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", modrinthUA)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		reachedAny = true
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			continue
		}
		var parsed map[string]modrinthVersion
		if err := json.Unmarshal(data, &parsed); err != nil {
			continue
		}
		for k, v := range parsed {
			result[k] = v
		}
		time.Sleep(250 * time.Millisecond) // be gentle with the public API
	}
	return result, reachedAny
}
