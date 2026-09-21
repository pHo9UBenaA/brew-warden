package homebrew

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBottleRegistrationTracksDigestChanges(t *testing.T) {
	now := int64(1800000000)
	for _, rebuilt := range []bool{false, true} {
		candidate := metadataFixture().Formulae[0]
		latest := "bottle do\n sha256 arm64_tahoe: \"" + string(candidate.BottleSHA256) + "\"\nend\n"
		older := latest
		if rebuilt {
			older = "bottle do\n sha256 arm64_tahoe: \"different\"\nend\n"
		}
		candidate.RecipeSHA256 = digestBytes([]byte(latest))
		commits := []registrationCommit{{SHA: strings.Repeat("a", 40)}, {SHA: strings.Repeat("b", 40)}, {SHA: strings.Repeat("c", 40)}}
		for i := range commits {
			commits[i].Commit.Committer.Date = time.Unix(now-int64(i*29+1)*86400, 0).UTC().Format(time.RFC3339)
		}
		history, _ := json.Marshal(commits)
		client := &http.Client{Transport: advisoryTransport(func(r *http.Request) (*http.Response, error) {
			body := ""
			switch r.URL.Host {
			case "api.github.com":
				if r.URL.Query().Get("sha") != candidate.TapCommit || r.URL.Query().Get("path") != candidate.RecipePath {
					t.Fatal("unbound history query")
				}
				body = string(history)
			case "raw.githubusercontent.com":
				switch {
				case strings.Contains(r.URL.Path, commits[0].SHA):
					body = latest
				case strings.Contains(r.URL.Path, commits[1].SHA):
					body = older
				default:
					body = "old bottle without this digest"
				}
			default:
				t.Fatal("unexpected host")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		e, raw, err := bottleRegistration(context.Background(), client, candidate, now)
		if err != nil {
			t.Fatal(err)
		}
		want := now - 30*86400
		if rebuilt {
			want = now - 86400
		}
		if e.PublishedAt != want || e.Subject != candidate.artifact() {
			t.Fatal("wrong bottle age", e.PublishedAt, want)
		}
		var observation registrationObservation
		if err := json.Unmarshal(raw, &observation); err != nil || !observation.ExactChange || len(observation.Recipes) == 0 {
			t.Fatal(err)
		}
	}
}

func TestBottleRegistrationRejectsUnboundHistory(t *testing.T) {
	now := int64(1800000000)
	for _, scenario := range []string{"wrong recipe", "absent digest", "future date", "clock inversion", "unavailable", "truncated"} {
		t.Run(scenario, func(t *testing.T) {
			candidate := metadataFixture().Formulae[0]
			recipe := "bottle " + string(candidate.BottleSHA256)
			if scenario == "absent digest" {
				recipe = "different bottle"
			}
			candidate.RecipeSHA256 = digestBytes([]byte(recipe))
			if scenario == "wrong recipe" {
				recipe += " changed"
			}
			commits := []registrationCommit{{SHA: strings.Repeat("a", 40)}, {SHA: strings.Repeat("b", 40)}}
			commits[0].Commit.Committer.Date = time.Unix(now-86400, 0).UTC().Format(time.RFC3339)
			commits[1].Commit.Committer.Date = time.Unix(now-172800, 0).UTC().Format(time.RFC3339)
			if scenario == "future date" {
				commits[0].Commit.Committer.Date = time.Unix(now+1, 0).UTC().Format(time.RFC3339)
			}
			if scenario == "clock inversion" {
				commits[1].Commit.Committer.Date = time.Unix(now-1, 0).UTC().Format(time.RFC3339)
			}
			history, _ := json.Marshal(commits)
			client := &http.Client{Transport: advisoryTransport(func(r *http.Request) (*http.Response, error) {
				body, status := recipe, 200
				if r.URL.Host == "api.github.com" {
					body = string(history)
					if scenario == "unavailable" {
						status = 503
					}
					if scenario == "truncated" {
						body = body[:len(body)-1]
					}
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			if _, _, err := bottleRegistration(context.Background(), client, candidate, now); err == nil {
				t.Fatal("unbound registration accepted")
			}
		})
	}
}
