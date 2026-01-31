package runner

import (
	"context"

	"github.com/will-x86/gcrawler"
	"github.com/will-x86/gcrawler/storage"
)

type Runner interface {
	Run(ctx context.Context, c crawler.Crawler, q storage.Queue) error
}
