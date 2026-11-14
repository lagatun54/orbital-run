//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type postgresOutage struct {
	project      string
	scoresID     string
	scoresBefore string
}

func stopPostgres(t *testing.T, project string) *postgresOutage {
	t.Helper()
	scoresID := composeOutput(t, project, "ps", "--quiet", "scores")
	if scoresID == "" {
		t.Fatal("scores container is missing from the test project")
	}
	outage := &postgresOutage{
		project:      project,
		scoresID:     scoresID,
		scoresBefore: scoresProcess(t, scoresID),
	}
	t.Log("stopping PostgreSQL before the match")
	composeOutput(t, project, "stop", "postgres")
	return outage
}

func (outage *postgresOutage) recover(t *testing.T, api backend, resultID string) {
	t.Helper()
	res := api.request(t, t.Context(), http.MethodGet, "/api/results/"+resultID, "", http.StatusServiceUnavailable, nil)
	res.Body.Close()
	outage.waitForFailedWrite(t, resultID)

	// Keep PostgreSQL down long enough for the Kafka consumer's 12-second write budget to expire.
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}

	t.Log("starting PostgreSQL and checking that scores recovered in the same process")
	composeOutput(t, outage.project, "start", "--wait", "--wait-timeout", "25", "postgres")
	if state := scoresProcess(t, outage.scoresID); state != outage.scoresBefore {
		t.Fatalf("scores changed process during the PostgreSQL outage: before %q, after %q", outage.scoresBefore, state)
	}
}

func (outage *postgresOutage) waitForFailedWrite(t *testing.T, resultID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		logs := dockerOutput(t, ctx, "logs", "--tail", "50", outage.scoresID)
		if strings.Contains(logs, "store Kafka match result: store match "+resultID+":") {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("scores did not attempt to store result %s while PostgreSQL was down", resultID)
		}
	}
}

func scoresProcess(t *testing.T, id string) string {
	t.Helper()
	return dockerOutput(t, t.Context(), "inspect", "--format", "{{.State.Status}} {{.State.StartedAt}}", id)
}

func composeOutput(t *testing.T, project string, args ...string) string {
	t.Helper()
	return dockerOutput(t, t.Context(), append([]string{"compose", "-p", project, "--project-directory", "../.."}, args...)...)
}

func dockerOutput(t *testing.T, parent context.Context, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
