package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func request(offset, count int) *http.Request {
	return httptest.NewRequest("GET", fmt.Sprintf(`/?offset=%d&count=%d`, offset, count), nil)
}

func run(t *testing.T, srv http.Handler, r *http.Request) (content []*Article) {
	response := httptest.NewRecorder()
	srv.ServeHTTP(response, r)

	if response.Code != 200 {
		t.Fatalf("Response code is %d, want 200", response.Code)
		return
	}

	decoder := json.NewDecoder(response.Body)
	err := decoder.Decode(&content)
	if err != nil {
		t.Fatalf("error decoding response body: %v", err)
	}

	return content
}

// Success cases

func TestResponse(t *testing.T) {
	content := run(t, app, request(0, 16))
	// check correct number of articles was returned
	assert.Len(t, content, 16)

	// check order of articles
	for i, item := range content {
		assert.Equal(t, Provider(item.Source), DefaultConfig[i%len(DefaultConfig)].Type)
	}
}

func TestNoQueryParams(t *testing.T) {
	content := run(t, app, request(0, 0))
	assert.Len(t, content, 0)
}

func TestNegativeCount(t *testing.T) {
	content := run(t, app, request(0, -10))

	assert.Len(t, content, 0)
}

// TestNegativeOffset also applies to the edge case when offset is 0 as the behavior will be identical
func TestNegativeOffset(t *testing.T) {
	content := run(t, app, request(-5, 10))

	// negative offset will be reset to 0 when extracting query params so it should not pose a threat
	assert.Len(t, content, 10)
}

func TestNegativeBothParams(t *testing.T) {
	content := run(t, app, request(-5, -10))

	assert.Len(t, content, 0)
}

// TestCountNotDivByProviders tests the edge case when, for example, there are 8 providers
// and we request 10 articles or 6 articles, so the count is not divisible by the number of providers,
// which should be handled correctly, by doing two batches for articles or respectively a single one
// that is cut off early
func TestCountNotDivByProviders(t *testing.T) {
	content := run(t, app, request(0, len(DefaultConfig)+2))
	assert.Len(t, content, len(DefaultConfig)+2)

	content = run(t, app, request(0, len(DefaultConfig)-2))
	assert.Len(t, content, len(DefaultConfig)-2)
}

// TestStress is used to assess performance and check for no crashing in cases of heavy loads
func TestStress(t *testing.T) {
	content := run(t, app, request(0, 99999))
	assert.Len(t, content, 99999)
}

// Error cases

func TestErr_AllProviders(t *testing.T) {
	errApp := App{
		ContentClients: map[Provider]Client{
			Provider1: ErrProvider{Source: Provider1},
			Provider2: ErrProvider{Source: Provider2},
			Provider3: ErrProvider{Source: Provider3},
		},
		Config: DefaultConfig,
	}

	// all getContents fail to return results so we'll have 0 results
	content := run(t, errApp, request(0, 16))

	assert.Len(t, content, 0)
}

func TestErr_NoFallback(t *testing.T) {
	errApp := App{
		ContentClients: map[Provider]Client{
			Provider1: ErrProvider{Source: Provider1},
			Provider2: SampleContentProvider{Source: Provider2},
			Provider3: SampleContentProvider{Source: Provider3},
		},
		Config: DefaultConfig,
	}

	// config1, config1, config2, config3, config4, config1, config1, config2,
	// 										  ^
	// when we try to get content from config4, we will find no fallback and failing
	// function, case in which we return the content retrieved so far, 4 articles in this case
	content := run(t, errApp, request(0, 16))

	assert.Len(t, content, 4)
}
