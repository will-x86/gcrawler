# loggers



There's a few default loggers, this tests all of them:


## Std Logger
This logger simply uses log.Printf("[DEBUG_LEVEL]"...)

## Slog Logger
Uses "log/slog" and levels

## ZeroLog Logger
Uses [zerolog](https://github.com/rs/zerlog) and two options:
1. Basic:
- Coloured output
- Default level to debug 
- Prints to stdout 
- Timefomat in minutes

2. Custom:
Allows for the following outputs to be set:
```go
type ZerologOptions struct {
	UseColor   bool
	Level      string
	TimeFormat string
	OutputFile string
}
```

If "OutputFile" is provided, logs will be directed to said file.


# Output:
```
=== Testing StdLogger ===
2026/01/31 13:29:03 [DEBUG] Worker 0: Processing https://example.com (active: 1)
2026/01/31 13:29:03 [DEBUG] Crawling: https://example.com
2026/01/31 13:29:03 [DEBUG] Processing URL: https://example.com
2026/01/31 13:29:03 [INFO] Crawled successfully: https://example.com
2026/01/31 13:29:03 [DEBUG] Worker 0: Queue empty
2026/01/31 13:29:03 [INFO] All workers finished

=== Testing SlogLogger ===
2026/01/31 13:29:03 [DEBUG] Queue empty and no active workers, finishing...
time=2026-01-31T13:29:04.360Z level=INFO msg="Crawled successfully: https://example.com"
time=2026-01-31T13:29:04.360Z level=INFO msg="All workers finished"

=== Testing ZerologLogger (basic) - coloured ===
13:29:05 DBG Worker 0: Processing https://example.com (active: 1)
13:29:05 DBG Crawling: https://example.com
13:29:05 DBG Processing URL: https://example.com
13:29:05 INF Crawled successfully: https://example.com
13:29:05 DBG Worker 0: Queue empty
13:29:05 INF All workers finished

=== Testing ZerologLogger (no-colour, debug, file-based) ===
13:29:05 DBG Queue empty and no active workers, finishing...
```

