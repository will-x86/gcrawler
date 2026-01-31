package runner

import (
	"context"
	"net/url"
	"testing"

	"github.com/will-x86/gcrawler/storage"
)

func TestPolicyAllowAll(t *testing.T) {
	policy := PolicyAllowAll
	currentURL, _ := url.Parse("https://example.com/page")

	tests := []string{
		"https://example.com/other",
		"https://different.com/page",
		"http://insecure.com",
		"https://subdomain.example.com/path",
	}

	for _, targetURL := range tests {
		if !policy.ShouldEnqueue(currentURL, targetURL) {
			t.Errorf("PolicyAllowAll should allow %q", targetURL)
		}
	}
}

func TestPolicyAllowNone(t *testing.T) {
	policy := PolicyAllowNone
	currentURL, _ := url.Parse("https://example.com/page")

	tests := []string{
		"https://example.com/other",
		"https://different.com/page",
	}

	for _, targetURL := range tests {
		if policy.ShouldEnqueue(currentURL, targetURL) {
			t.Errorf("PolicyAllowNone should reject %q", targetURL)
		}
	}
}

func TestPolicySameDomain(t *testing.T) {
	q := storage.NewMemoryQueue()
	ctx := context.Background()

	q.Add(ctx, "https://example.com/seed1")
	q.Add(ctx, "https://another.com/seed2")

	policy := &sameDomainPolicy{allowedHosts: make(map[string]bool)}
	if err := policy.Initialize(ctx, q); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	tests := []struct {
		currentURL string
		targetURL  string
		want       bool
	}{
		{"https://example.com/page1", "https://example.com/page2", true},
		{"https://example.com/page1", "https://different.com/page", false},
		{"https://another.com/page1", "https://another.com/page2", true},
		{"https://another.com/page1", "https://example.com/page", true},
		{"https://example.com/page", "https://subdomain.example.com/page", false},
	}

	for _, tt := range tests {
		currentURL, _ := url.Parse(tt.currentURL)
		got := policy.ShouldEnqueue(currentURL, tt.targetURL)
		if got != tt.want {
			t.Errorf("ShouldEnqueue(%q, %q) = %v, want %v", tt.currentURL, tt.targetURL, got, tt.want)
		}
	}
}

func TestGlobPolicy(t *testing.T) {
	tests := []struct {
		name       string
		patterns   []string
		currentURL string
		targetURL  string
		want       bool
	}{
		{
			name:       "single pattern match",
			patterns:   []string{"example.com"},
			currentURL: "https://example.com/page1",
			targetURL:  "https://example.com/page2",
			want:       true,
		},
		{
			name:       "single pattern no match",
			patterns:   []string{"example.com"},
			currentURL: "https://example.com/page1",
			targetURL:  "https://different.com/page",
			want:       false,
		},
		{
			name:       "wildcard pattern",
			patterns:   []string{"*.example.com"},
			currentURL: "https://example.com/page1",
			targetURL:  "https://sub.example.com/page",
			want:       true,
		},
		{
			name:       "negation pattern",
			patterns:   []string{"*.com", "!bad.com"},
			currentURL: "https://good.com/page",
			targetURL:  "https://bad.com/page",
			want:       false,
		},
		{
			name:       "negation doesn't match",
			patterns:   []string{"*.com", "!bad.com"},
			currentURL: "https://good.com/page",
			targetURL:  "https://good.com/other",
			want:       true,
		},
		{
			name:       "multiple patterns",
			patterns:   []string{"example.com", "test.com"},
			currentURL: "https://example.com/page",
			targetURL:  "https://test.com/page",
			want:       true,
		},
		{
			name:       "no patterns match",
			patterns:   []string{"example.com", "test.com"},
			currentURL: "https://example.com/page",
			targetURL:  "https://other.com/page",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := NewGlobPolicy(tt.patterns...)
			currentURL, _ := url.Parse(tt.currentURL)
			got := policy.ShouldEnqueue(currentURL, tt.targetURL)
			if got != tt.want {
				t.Errorf("ShouldEnqueue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMaxPerDomainPolicy(t *testing.T) {
	policy := NewMaxPerDomainPolicy(2)
	currentURL, _ := url.Parse("https://example.com/page1")

	tests := []struct {
		targetURL string
		want      bool
	}{
		{"https://example.com/page2", true},
		{"https://example.com/page3", true},
		{"https://example.com/page4", false},
		{"https://different.com/page1", true},
		{"https://different.com/page2", true},
		{"https://different.com/page3", false},
	}

	for i, tt := range tests {
		got := policy.ShouldEnqueue(currentURL, tt.targetURL)
		if got != tt.want {
			t.Errorf("test %d: ShouldEnqueue(%q) = %v, want %v", i, tt.targetURL, got, tt.want)
		}
	}
}

func TestMaxDepthPolicy(t *testing.T) {
	t.Run("depth limit enforced", func(t *testing.T) {
		policy := NewMaxDepthPolicy(2, nil)
		ctx := context.Background()
		q := storage.NewMemoryQueue()

		if err := policy.Initialize(ctx, q); err != nil {
			t.Fatalf("Initialize() error: %v", err)
		}

		seed, _ := url.Parse("https://example.com/seed")

		level1, _ := url.Parse("https://example.com/level1")
		if !policy.ShouldEnqueue(seed, level1.String()) {
			t.Error("should allow depth 1")
		}

		level2, _ := url.Parse("https://example.com/level2")
		if !policy.ShouldEnqueue(level1, level2.String()) {
			t.Error("should allow depth 2")
		}

		level3, _ := url.Parse("https://example.com/level3")
		if policy.ShouldEnqueue(level2, level3.String()) {
			t.Error("should reject depth 3 (exceeds max of 2)")
		}
	})

	t.Run("with inner policy", func(t *testing.T) {
		innerPolicy := NewGlobPolicy("example.com")
		policy := NewMaxDepthPolicy(5, innerPolicy)
		ctx := context.Background()
		q := storage.NewMemoryQueue()

		if err := policy.Initialize(ctx, q); err != nil {
			t.Fatalf("Initialize() error: %v", err)
		}

		seed, _ := url.Parse("https://example.com/seed")

		allowedURL := "https://example.com/allowed"
		if !policy.ShouldEnqueue(seed, allowedURL) {
			t.Error("should allow URL matching inner policy")
		}

		rejectedURL := "https://different.com/rejected"
		if policy.ShouldEnqueue(seed, rejectedURL) {
			t.Error("should reject URL not matching inner policy")
		}
	})

	t.Run("independent depth tracking per URL", func(t *testing.T) {
		policy := NewMaxDepthPolicy(1, nil)
		ctx := context.Background()
		q := storage.NewMemoryQueue()

		if err := policy.Initialize(ctx, q); err != nil {
			t.Fatalf("Initialize() error: %v", err)
		}

		seed1, _ := url.Parse("https://example.com/seed1")
		seed2, _ := url.Parse("https://example.com/seed2")

		child1, _ := url.Parse("https://example.com/child1")
		if !policy.ShouldEnqueue(seed1, child1.String()) {
			t.Error("should allow child from seed1")
		}

		child2, _ := url.Parse("https://example.com/child2")
		if !policy.ShouldEnqueue(seed2, child2.String()) {
			t.Error("should allow child from seed2 (independent depth)")
		}
	})
}

func TestMaxDepthPolicy_DepthZero(t *testing.T) {
	policy := NewMaxDepthPolicy(0, nil)
	ctx := context.Background()
	q := storage.NewMemoryQueue()

	if err := policy.Initialize(ctx, q); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	seed, _ := url.Parse("https://example.com/seed")
	child := "https://example.com/child"

	if policy.ShouldEnqueue(seed, child) {
		t.Error("max depth 0 should reject any child URLs")
	}
}

func TestLinkPolicy_InvalidURLs(t *testing.T) {
	policies := []struct {
		name   string
		policy LinkPolicy
	}{
		{"AllowAll", PolicyAllowAll},
		{"AllowNone", PolicyAllowNone},
		{"SameDomain", PolicySameDomain},
		{"Glob", NewGlobPolicy("*.com")},
		{"MaxPerDomain", NewMaxPerDomainPolicy(10)},
		{"MaxDepth", NewMaxDepthPolicy(5, nil)},
	}

	currentURL, _ := url.Parse("https://example.com/page")
	invalidURLs := []string{
		"://invalid",
		"not-a-url",
		"",
	}

	for _, p := range policies {
		t.Run(p.name, func(t *testing.T) {
			for _, invalidURL := range invalidURLs {
				result := p.policy.ShouldEnqueue(currentURL, invalidURL)
				if result && p.name != "AllowAll" {
					t.Errorf("%s should reject invalid URL %q", p.name, invalidURL)
				}
			}
		})
	}
}

func TestLinkPolicy_Initialize(t *testing.T) {
	policies := []LinkPolicy{
		PolicyAllowAll,
		PolicyAllowNone,
		PolicySameDomain,
		NewGlobPolicy("*.com"),
		NewMaxPerDomainPolicy(10),
		NewMaxDepthPolicy(5, nil),
	}

	ctx := context.Background()
	q := storage.NewMemoryQueue()

	for i, policy := range policies {
		if err := policy.Initialize(ctx, q); err != nil {
			t.Errorf("policy %d Initialize() error: %v", i, err)
		}
	}
}

func TestSameDomainPolicy_EmptyQueue(t *testing.T) {
	q := storage.NewMemoryQueue()
	ctx := context.Background()

	policy := &sameDomainPolicy{allowedHosts: make(map[string]bool)}
	if err := policy.Initialize(ctx, q); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}

	currentURL, _ := url.Parse("https://example.com/page")
	if policy.ShouldEnqueue(currentURL, "https://example.com/other") {
		t.Error("should reject all URLs when no hosts are initialized")
	}
}
