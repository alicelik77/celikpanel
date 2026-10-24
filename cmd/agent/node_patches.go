package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Node.js patch notices (alpha.107, D-038 §9). The panel installs Node.js
// from its official releases, so the operating system never updates it: for
// every installed version, this reads the official release index (kept six
// hours) and says the newest release on the same line. Installing it is the
// existing verified install (SHASUMS256); moving a site to it is the
// administrator's request, one site at a time, with proof.
// Node.js yama bildirimleri: kurulu her sürüm için aynı hattın en yenisi.

const nodeIndexCache = 6 * time.Hour

var nodeIndexState = struct {
	sync.Mutex
	at   time.Time
	body []byte
}{}

var fetchNodeIndex = func(ctx context.Context) ([]byte, time.Time, error) {
	nodeIndexState.Lock()
	defer nodeIndexState.Unlock()
	if nodeIndexState.body != nil && time.Since(nodeIndexState.at) < nodeIndexCache {
		return nodeIndexState.body, nodeIndexState.at, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://nodejs.org/dist/index.json", nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, time.Time{}, err
	}
	nodeIndexState.at, nodeIndexState.body = time.Now(), body
	return body, nodeIndexState.at, nil
}

func nodeLine(version string) string {
	version = strings.TrimPrefix(version, "v")
	if i := strings.IndexByte(version, '.'); i > 0 {
		return version[:i]
	}
	return ""
}

// nodePatchesFrom reads the index (newest first) for every installed version.
func nodePatchesFrom(index []byte, installed []string) ([]transport.NodePatch, error) {
	var releases []struct {
		Version string          `json:"version"`
		LTS     json.RawMessage `json:"lts"`
	}
	if err := json.Unmarshal(index, &releases); err != nil {
		return nil, fmt.Errorf("release index unreadable: %v", err)
	}
	newest := map[string]string{}
	lts := map[string]string{}
	for _, release := range releases {
		version := strings.TrimPrefix(release.Version, "v")
		line := nodeLine(version)
		if line == "" || !nodeVersionRe.MatchString(version) {
			continue
		}
		if _, seen := newest[line]; !seen {
			newest[line] = version
		}
		var name string
		if json.Unmarshal(release.LTS, &name) == nil && name != "" && lts[line] == "" {
			lts[line] = name
		}
	}
	out := []transport.NodePatch{}
	for _, version := range installed {
		line := nodeLine(version)
		patch := transport.NodePatch{Installed: version, Line: line, Newest: newest[line], LTS: lts[line]}
		patch.Newer = patch.Newest != "" && nodeVersionLess(version, patch.Newest)
		out = append(out, patch)
	}
	return out, nil
}

func nodeVersionLess(a, b string) bool {
	var pa, pb [3]int
	fmt.Sscanf(a, "%d.%d.%d", &pa[0], &pa[1], &pa[2])
	fmt.Sscanf(b, "%d.%d.%d", &pb[0], &pb[1], &pb[2])
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

// NodePatchesV1 answers, read only, the newest release on each installed
// Node.js line.
func (a *Agent) NodePatchesV1(_ *transport.Empty, resp *transport.NodePatchesResponse) error {
	*resp = transport.NodePatchesResponse{Patches: []transport.NodePatch{}}
	var installed transport.NodeVersionsResponse
	if err := a.ListNodeVersions(&transport.Empty{}, &installed); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if len(installed.Installed) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	index, at, err := fetchNodeIndex(ctx)
	if err != nil {
		resp.Error = "the Node.js release index could not be read: " + err.Error()
		return nil
	}
	resp.CheckedAt = at.UTC().Format(time.RFC3339)
	patches, err := nodePatchesFrom(index, installed.Installed)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.Patches = patches
	return nil
}
