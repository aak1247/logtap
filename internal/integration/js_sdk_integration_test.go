package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/testkit"
)

func TestIntegration_JSSDK_Gateway_SQLite(t *testing.T) {
	t.Parallel()

	srv := testkit.NewServer(t)
	client := srv.HTTP.Client()
	baseURL := srv.HTTP.URL

	boot := testkit.Bootstrap(t, client, baseURL)

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH")
	}

	repoRoot := testkit.RepoRoot(t)

	checks := [][]string{
		{node, "-e", "process.exit(typeof fetch === 'function' ? 0 : 1)"},
	}
	for _, args := range checks {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("node does not meet requirements: %v (%s)", err, string(out))
		}
	}

	runID := strconv.FormatInt(time.Now().UnixNano(), 10)
	msg := "js-sdk-e2e-" + runID
	ev := "js-sdk-signup-" + runID

	integrationScript := filepath.Join(repoRoot, "sdks", "js", "logtap", "test", "integration.test.mjs")
	batchingScript := filepath.Join(repoRoot, "sdks", "js", "logtap", "test", "batching.test.mjs")

	cmd := exec.Command(node, integrationScript)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"LOGTAP_BASE_URL="+baseURL,
		fmt.Sprintf("LOGTAP_PROJECT_ID=%d", boot.ProjectID),
		"LOGTAP_PROJECT_KEY="+boot.ProjectKey,
		"LOGTAP_TEST_MESSAGE="+msg,
		"LOGTAP_TEST_EVENT="+ev,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("node integration test failed: %v\n%s", err, string(out))
	}

	cmd = exec.Command(node, batchingScript)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("node batching test failed: %v\n%s", err, string(out))
	}

	logs := testkit.SearchLogs(t, client, baseURL, boot.Token, boot.ProjectID, testkit.SearchLogsParams{Limit: 50})
	hasInfo := false
	hasEvent := false
	for _, row := range logs {
		m, _ := row["message"].(string)
		l, _ := row["level"].(string)
		if m == msg && l == "info" {
			hasInfo = true
		}
		if m == ev && l == "event" {
			hasEvent = true
		}
	}
	if !hasInfo {
		t.Fatalf("expected js info log not found (logs=%v)", logs)
	}
	if !hasEvent {
		t.Fatalf("expected js event log not found (logs=%v)", logs)
	}
}
