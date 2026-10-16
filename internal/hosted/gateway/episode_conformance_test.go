package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type episodeOfflineSchemaLoader struct{}

func (episodeOfflineSchemaLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema disabled: %s", url)
}
func episodeFixtureSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/protocol/schemas/episodes/episode_" + name + ".schema.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(episodeOfflineSchemaLoader{})
	id := "urn:serenity:episodes:" + name + ":v1"
	if err = compiler.AddResource(id, doc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(id)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}
func TestEpisodeFrozenHTTPConformance(t *testing.T) {
	g, _, brain, token := episodeHTTPSetup(t)
	requestSchema, responseSchema, errorSchema := episodeFixtureSchema(t, "request"), episodeFixtureSchema(t, "response"), episodeFixtureSchema(t, "error")
	raw, err := os.ReadFile("../../../testdata/conformance/episodes/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name     string          `json:"name"`
			Request  json.RawMessage `json:"request"`
			Status   int             `json:"http_status"`
			Expected map[string]any  `json:"expected"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) != 14 {
		t.Fatal("unexpected conformance inventory")
	}
	var firstReceipt any
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			body := strings.ReplaceAll(string(tc.Request), "$brain", brain)
			value, err := jsonschema.UnmarshalJSON(strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			if err = requestSchema.Validate(value); err != nil {
				t.Fatalf("request schema: %v", err)
			}
			req := httptest.NewRequest("POST", "/episodes/v1", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			g.ServeEpisodes(w, req)
			if w.Code != tc.Status {
				t.Fatalf("HTTP %d expected %d: %s", w.Code, tc.Status, w.Body.String())
			}
			value, err = jsonschema.UnmarshalJSON(bytes.NewReader(w.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			schema := responseSchema
			if w.Code != 200 {
				schema = errorSchema
			}
			if err = schema.Validate(value); err != nil {
				t.Fatalf("response schema: %v\n%s", err, w.Body.String())
			}
			var response map[string]any
			if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			for key, want := range tc.Expected {
				if !reflect.DeepEqual(response[key], want) {
					t.Fatalf("%s=%v want %v", key, response[key], want)
				}
			}
			if tc.Name == "append" {
				firstReceipt = response["receipt"]
			}
			if tc.Name == "identical-retry" && !reflect.DeepEqual(firstReceipt, response["receipt"]) {
				t.Fatal("receipt changed")
			}
		})
	}
}
