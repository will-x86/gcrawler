
# Running:



This basic example uses:



- SQLite Queue
- Memory Based Storage
- Standard Logger (StdLogger)
- SimpleHTTPCrawler
- Basic Async Runner 


Then it:
1. Crawls all URL's in the queue (only one for now) 
2. Get's the byte value of the HTML 
3. Stores this in the storage.



The end result of running this will be something like:

```
2026/01/31 13:22:05 [DEBUG] Worker 0: Processing https://example.com (active: 1)
2026/01/31 13:22:05 [DEBUG] Crawling: https://example.com
2026/01/31 13:22:05 [INFO] Crawled https://example.com
2026/01/31 13:22:05 [DEBUG] Worker 0: Queue empty
2026/01/31 13:22:06 [DEBUG] Worker 1: Queue empty
2026/01/31 13:22:06 [INFO] All workers finished

Queue stats: map[completed:1]
```


The SQLite table will be as such:
```sql
sqlite> .schema
CREATE TABLE requests (
		id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		unique_key TEXT NOT NULL UNIQUE,
		status TEXT NOT NULL DEFAULT 'pending',
		retry_count INTEGER NOT NULL DEFAULT 0,
		added_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		last_error TEXT
	);
CREATE INDEX idx_status ON requests(status);
CREATE INDEX idx_unique_key ON requests(unique_key);
CREATE INDEX idx_status_retry ON requests(status, retry_count);
sqlite>
```

Data as such:
```
100680ad546ce6a5|https://example.com|https://example.com|completed|0|2026-01-31 13:18:25.432062143+00:00|2026-01-31 13:18:26.515997607+00:00|
```

