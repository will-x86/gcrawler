package runner

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/will-x86/gcrawler/storage"
)

type LinkPolicy interface {
	ShouldEnqueue(currentURL *url.URL, targetURL string) bool
	Initialize(ctx context.Context, queue storage.Queue) error
}

type allowAllPolicy struct{}

func (p *allowAllPolicy) ShouldEnqueue(currentURL *url.URL, targetURL string) bool {
	return true
}

func (p *allowAllPolicy) Initialize(ctx context.Context, queue storage.Queue) error {
	return nil
}

type allowNonePolicy struct{}

func (p *allowNonePolicy) ShouldEnqueue(currentURL *url.URL, targetURL string) bool {
	return false
}

func (p *allowNonePolicy) Initialize(ctx context.Context, queue storage.Queue) error {
	return nil
}

type sameDomainPolicy struct {
	allowedHosts map[string]bool
}

func (p *sameDomainPolicy) ShouldEnqueue(currentURL *url.URL, targetURL string) bool {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	return p.allowedHosts[parsed.Host]
}

func (p *sameDomainPolicy) Initialize(ctx context.Context, queue storage.Queue) error {
	hosts, err := queue.GetAllHosts(ctx)
	if err != nil {
		return err
	}

	p.allowedHosts = make(map[string]bool)
	for _, host := range hosts {
		p.allowedHosts[host] = true
	}
	return nil
}

type globPolicy struct {
	patterns []string
}

func (p *globPolicy) ShouldEnqueue(currentURL *url.URL, targetURL string) bool {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}

	targetHost := parsed.Host
	allowed := false

	for _, pattern := range p.patterns {
		negPattern, found := strings.CutPrefix(pattern, "!")
		if found {
			/*if strings.HasPrefix(pattern, "!") {
			negPattern := strings.TrimPrefix(pattern, "!")*/
			if matchHost(targetHost, negPattern) {
				return false
			}
		} else {
			if matchHost(targetHost, pattern) {
				allowed = true
			}
		}
	}

	return allowed
}

func (p *globPolicy) Initialize(ctx context.Context, queue storage.Queue) error {
	return nil
}

func matchHost(host, pattern string) bool {
	matched, err := filepath.Match(pattern, host)
	if err != nil {
		return false
	}
	return matched
}

type maxPerDomainPolicy struct {
	maxPerDomain int
	domainCounts map[string]int
	mu           sync.Mutex
}

func (p *maxPerDomainPolicy) ShouldEnqueue(currentURL *url.URL, targetURL string) bool {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}

	host := parsed.Host
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.domainCounts[host] >= p.maxPerDomain {
		return false
	}

	p.domainCounts[host]++
	return true
}

func (p *maxPerDomainPolicy) Initialize(ctx context.Context, queue storage.Queue) error {
	return nil
}

var (
	PolicyAllowAll   LinkPolicy = &allowAllPolicy{}
	PolicyAllowNone  LinkPolicy = &allowNonePolicy{}
	PolicySameDomain LinkPolicy = &sameDomainPolicy{allowedHosts: make(map[string]bool)}
)

func NewGlobPolicy(patterns ...string) LinkPolicy {
	return &globPolicy{
		patterns: patterns,
	}
}

func NewMaxPerDomainPolicy(maxPerDomain int) LinkPolicy {
	return &maxPerDomainPolicy{
		maxPerDomain: maxPerDomain,
		domainCounts: make(map[string]int),
	}
}

type maxDepthPolicy struct {
	maxDepth   int
	urlDepths  map[string]int
	innerPolicy LinkPolicy
	mu         sync.Mutex
}

func (p *maxDepthPolicy) ShouldEnqueue(currentURL *url.URL, targetURL string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Get current URL's depth (defaults to 0 for seeds)
	currentDepth := p.urlDepths[currentURL.String()]

	// Calculate target depth
	targetDepth := currentDepth + 1

	// Check if target depth exceeds max
	if targetDepth > p.maxDepth {
		return false
	}

	// Check inner policy if provided
	if p.innerPolicy != nil && !p.innerPolicy.ShouldEnqueue(currentURL, targetURL) {
		return false
	}

	// Store the target URL's depth
	p.urlDepths[targetURL] = targetDepth
	return true
}

func (p *maxDepthPolicy) Initialize(ctx context.Context, queue storage.Queue) error {
	// Seed URLs will be initialized with depth 0 on first encounter in ShouldEnqueue
	if p.innerPolicy != nil {
		return p.innerPolicy.Initialize(ctx, queue)
	}
	return nil
}

func NewMaxDepthPolicy(maxDepth int, innerPolicy LinkPolicy) LinkPolicy {
	return &maxDepthPolicy{
		maxDepth:    maxDepth,
		urlDepths:   make(map[string]int),
		innerPolicy: innerPolicy,
	}
}
