// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package githubreceiver

import (
	"testing"

	"github.com/google/go-github/v92/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestFormatString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "converts underscores to hyphens",
			input:    "hello_world",
			expected: "hello-world",
		},
		{
			name:     "converts to lowercase",
			input:    "HELLO_WORLD",
			expected: "hello-world",
		},
		{
			name:     "handles mixed case and multiple underscores",
			input:    "Hello_Big_WORLD",
			expected: "hello-big-world",
		},
		{
			name:     "handles string with no underscores",
			input:    "HelloWorld",
			expected: "helloworld",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatString(tt.input)
			if got != tt.expected {
				t.Errorf("formatString() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSplitRefWorkflowPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple workflow with version",
			input:    "my-great-workflow@v1.0.0",
			expected: "my-great-workflow",
			wantErr:  false,
		},
		{
			name:     "workflow with SHA",
			input:    "my-great-workflow@3421498310493281409328140932840192384",
			expected: "my-great-workflow",
			wantErr:  false,
		},
		{
			name:     "full path workflow",
			input:    "org/repo/.github/my-file-path/with/folder/build-woot.yaml@v0.2.3",
			expected: "build-woot",
			wantErr:  false,
		},
		{
			name:     "uppercase file",
			input:    "org/repo/.github/my-file-path/with/folder/BUILD-WOOT.yaml@v0.2.3",
			expected: "build-woot",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := splitRefWorkflowPath(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestReplaceAPIURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "converts api.github.com URL to html URL",
			input:    "https://api.github.com/repos/open-telemetry/opentelemetry-collector-contrib/pull/1234",
			expected: "https://github.com/open-telemetry/opentelemetry-collector-contrib/pull/1234",
		},
		{
			name:     "converts api.github.com workflow URL to html URL",
			input:    "https://api.github.com/repos/open-telemetry/opentelemetry-collector-contrib/actions/runs/1234",
			expected: "https://github.com/open-telemetry/opentelemetry-collector-contrib/actions/runs/1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceAPIURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPreviousAttemptHTMLURL(t *testing.T) {
	const githubHTML = "https://github.com/open-telemetry/open-telemetry-otel-collector/actions/runs/14460881260"
	const githubAPI = "https://api.github.com/repos/open-telemetry/open-telemetry-otel-collector/actions/runs/14460881260/attempts/12"
	const enterpriseHTML = "https://github.acme.corp/platform/payments-service/actions/runs/998877"
	const enterpriseAPI = "https://github.acme.corp/api/v3/repos/platform/payments-service/actions/runs/998877/attempts/2"

	tests := []struct {
		name     string
		htmlURL  string
		attempt  int
		apiURL   string
		expected string
	}{
		{
			name:     "github.com uses html_url and the previous attempt number",
			htmlURL:  githubHTML,
			attempt:  13,
			apiURL:   githubAPI,
			expected: githubHTML + "/attempts/12",
		},
		{
			name:     "enterprise server uses the html_url host instead of the api path",
			htmlURL:  enterpriseHTML,
			attempt:  3,
			apiURL:   enterpriseAPI,
			expected: enterpriseHTML + "/attempts/2",
		},
		{
			name:     "trims a trailing slash on html_url",
			htmlURL:  enterpriseHTML + "/",
			attempt:  3,
			apiURL:   enterpriseAPI,
			expected: enterpriseHTML + "/attempts/2",
		},
		{
			name:     "falls back to github.com api rewrite when html_url is missing",
			htmlURL:  "",
			attempt:  13,
			apiURL:   githubAPI,
			expected: "https://github.com/open-telemetry/open-telemetry-otel-collector/actions/runs/14460881260/attempts/12",
		},
		{
			name:     "first attempt has no previous url",
			htmlURL:  githubHTML,
			attempt:  1,
			apiURL:   "",
			expected: "",
		},
		{
			name:     "missing previous attempt api url is omitted",
			htmlURL:  enterpriseHTML,
			attempt:  3,
			apiURL:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, previousAttemptHTMLURL(tt.htmlURL, tt.attempt, tt.apiURL))
		})
	}
}

func TestGetWorkflowRunAttrsPreviousAttemptEnterprise(t *testing.T) {
	htmlURL := "https://github.acme.corp/platform/payments-service/actions/runs/998877"
	previousAPI := "https://github.acme.corp/api/v3/repos/platform/payments-service/actions/runs/998877/attempts/2"
	repoName := "payments-service"
	event := &github.WorkflowRunEvent{
		WorkflowRun: &github.WorkflowRun{
			Name:               github.Ptr("build-and-test"),
			HeadBranch:         github.Ptr("main"),
			HeadSHA:            github.Ptr("abc123"),
			ID:                 github.Ptr(int64(998877)),
			Conclusion:         github.Ptr("failure"),
			HTMLURL:            &htmlURL,
			RunAttempt:         github.Ptr(3),
			PreviousAttemptURL: &previousAPI,
		},
		Repo:   &github.Repository{Name: &repoName},
		Sender: &github.User{Login: github.Ptr("octocat")},
	}

	gtr := &githubTracesReceiver{cfg: &Config{}}
	resource := pcommon.NewResource()
	require.NoError(t, gtr.getWorkflowRunAttrs(resource, event))

	got, ok := resource.Attributes().Get(AttributeCICDPipelinePreviousAttemptURLFull)
	require.True(t, ok)
	assert.Equal(t, htmlURL+"/attempts/2", got.Str())
}

func TestAddCustomPropertiesToAttrs(t *testing.T) {
	tests := []struct {
		name         string
		customProps  map[string]any
		expectedKeys []string
		expectedVals map[string]any
	}{
		{
			name: "adds string custom properties",
			customProps: map[string]any{
				"team_name":    "open-telemetry",
				"environment":  "development",
				"service_name": "should-be-skipped", // This should be skipped
			},
			expectedKeys: []string{
				"github.repository.custom_properties.team_name",
				"github.repository.custom_properties.environment",
			},
			expectedVals: map[string]any{
				"github.repository.custom_properties.team_name":   "open-telemetry",
				"github.repository.custom_properties.environment": "development",
			},
		},
		{
			name: "adds different types of custom properties",
			customProps: map[string]any{
				"string_prop": "string-value",
				"int_prop":    42,
				"float_prop":  3.14,
				"bool_prop":   true,
			},
			expectedKeys: []string{
				"github.repository.custom_properties.string_prop",
				"github.repository.custom_properties.int_prop",
				"github.repository.custom_properties.float_prop",
				"github.repository.custom_properties.bool_prop",
			},
			expectedVals: map[string]any{
				"github.repository.custom_properties.string_prop": "string-value",
				"github.repository.custom_properties.int_prop":    int64(42),
				"github.repository.custom_properties.float_prop":  3.14,
				"github.repository.custom_properties.bool_prop":   true,
			},
		},
		{
			name: "converts keys to snake_case",
			customProps: map[string]any{
				"camelCase":   "camel-value",
				"PascalCase":  "pascal-value",
				"kebab-case":  "kebab-value",
				"space case":  "space-value",
				"mixed_Case":  "mixed-value",
				"withNumber1": "number-value",
				"with.dots":   "dots-value",
				"$dollar":     "dollar-value",
				"#hash":       "hash-value",
			},
			expectedKeys: []string{
				"github.repository.custom_properties.camel_case",
				"github.repository.custom_properties.pascal_case",
				"github.repository.custom_properties.kebab_case",
				"github.repository.custom_properties.space_case",
				"github.repository.custom_properties.mixed_case",
				"github.repository.custom_properties.with_number1",
				"github.repository.custom_properties.with_dots",
				"github.repository.custom_properties._dollar_dollar",
				"github.repository.custom_properties._hash_hash",
			},
			expectedVals: map[string]any{
				"github.repository.custom_properties.camel_case":     "camel-value",
				"github.repository.custom_properties.pascal_case":    "pascal-value",
				"github.repository.custom_properties.kebab_case":     "kebab-value",
				"github.repository.custom_properties.space_case":     "space-value",
				"github.repository.custom_properties.mixed_case":     "mixed-value",
				"github.repository.custom_properties.with_number1":   "number-value",
				"github.repository.custom_properties.with_dots":      "dots-value",
				"github.repository.custom_properties._dollar_dollar": "dollar-value",
				"github.repository.custom_properties._hash_hash":     "hash-value",
			},
		},
		{
			name:         "handles empty custom properties",
			customProps:  map[string]any{},
			expectedKeys: []string{},
			expectedVals: map[string]any{},
		},
		{
			name:         "handles nil custom properties",
			customProps:  nil,
			expectedKeys: []string{},
			expectedVals: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new resource with empty attributes
			resource := pcommon.NewResource()
			attrs := resource.Attributes()

			// Add custom properties to attributes
			addCustomPropertiesToAttrs(attrs, tt.customProps)

			// Check that all expected keys exist
			for _, key := range tt.expectedKeys {
				_, exists := attrs.Get(key)
				assert.True(t, exists, "Expected key %s not found", key)
			}

			// Check that service_name is not added
			_, exists := attrs.Get("github.repository.custom_properties.service_name")
			assert.False(t, exists)

			// Check values
			for key, expectedVal := range tt.expectedVals {
				switch expected := expectedVal.(type) {
				case string:
					val, _ := attrs.Get(key)
					assert.Equal(t, expected, val.Str())
				case int64:
					val, _ := attrs.Get(key)
					assert.Equal(t, expected, val.Int())
				case float64:
					val, _ := attrs.Get(key)
					assert.Equal(t, expected, val.Double())
				case bool:
					val, _ := attrs.Get(key)
					assert.Equal(t, expected, val.Bool())
				}
			}

			// Check that no unexpected keys were added
			count := 0
			attrs.Range(func(_ string, _ pcommon.Value) bool {
				count++
				return true
			})
			assert.Equal(t, len(tt.expectedKeys), count)
		})
	}
}
